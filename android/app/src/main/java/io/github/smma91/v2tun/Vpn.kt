package io.github.smma91.v2tun

import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.core.content.ContextCompat
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

enum class Status { STOPPED, STARTING, STARTED, STOPPING }

/** The VPN's state, shared by the service, the screen, the tile and the notification. */
object Vpn {
    private val _status = MutableStateFlow(Status.STOPPED)
    val status: StateFlow<Status> = _status

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error

    var since = 0L
        private set

    internal fun set(s: Status, error: String? = null) {
        if (s == Status.STARTED) since = System.currentTimeMillis()
        _status.value = s
        if (error != null || s == Status.STARTING) _error.value = error
    }

    /** Android must have granted the VPN permission (VpnService.prepare) before. */
    fun start(ctx: Context) {
        if (VpnService.prepare(ctx) != null) return
        set(Status.STARTING)
        ContextCompat.startForegroundService(
            ctx, Intent(ctx, V2tunVpnService::class.java).setAction(V2tunVpnService.ACTION_START)
        )
    }

    fun stop(ctx: Context) {
        if (_status.value == Status.STOPPED) return
        ctx.startService(Intent(ctx, V2tunVpnService::class.java).setAction(V2tunVpnService.ACTION_STOP))
    }
}
