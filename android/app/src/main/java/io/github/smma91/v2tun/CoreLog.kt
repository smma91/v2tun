package io.github.smma91.v2tun

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** One line of the core's log. [level] uses sing-box's levels (0 panic … 6 trace). */
data class LogLine(val id: Long, val level: Int, val text: String)

/**
 * The last lines of the core's log, kept in memory for the Log screen. The core
 * is only asked for its log while that screen is open (battery).
 */
object CoreLog {
    const val MAX_LINES = 300
    const val LEVEL_ERROR = 2
    const val LEVEL_WARN = 3
    const val LEVEL_INFO = 4

    private val ansi = Regex("\u001B\\[[0-9;]*[A-Za-z]")
    private var nextId = 0L
    private var replaying = false

    private val _lines = MutableStateFlow<List<LogLine>>(emptyList())
    /** Every level (the core's debug lines push older ones out quickly). */
    val lines: StateFlow<List<LogLine>> = _lines

    private val _important = MutableStateFlow<List<LogLine>>(emptyList())
    /** Warnings and errors only, kept separately so they are not pushed out. */
    val important: StateFlow<List<LogLine>> = _important

    /** Whether the Log screen is open (the service then subscribes to the core's log). */
    var visible = false
        set(v) {
            field = v
            V2tunVpnService.instance?.updateWatching()
        }

    @Synchronized
    fun add(entries: List<Pair<Int, String>>) {
        val added = entries.mapNotNull { (level, msg) ->
            val text = msg.replace(ansi, "").trimEnd()
            if (text.isEmpty()) null else LogLine(nextId++, level, text)
        }
        if (added.isEmpty()) return
        _lines.value = (_lines.value + added).takeLast(MAX_LINES)
        var warnings = added.filter { it.level <= LEVEL_WARN }
        if (replaying) {
            // The core sends the lines it kept again on every subscription.
            val known = _important.value.mapTo(HashSet()) { it.text }
            warnings = warnings.filter { it.text !in known }
            replaying = false
        }
        if (warnings.isNotEmpty()) _important.value = (_important.value + warnings).takeLast(MAX_LINES)
    }

    /** The core is about to send the lines it kept (a new subscription). */
    @Synchronized
    fun reset() {
        _lines.value = emptyList()
        replaying = true
    }

    @Synchronized
    fun clear() {
        _lines.value = emptyList()
        _important.value = emptyList()
    }
}
