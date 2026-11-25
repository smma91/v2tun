package io.github.smma91.v2tun

import io.github.smma91.v2tun.mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

data class ServerItem(
    val id: String, val sub: String, val name: String, val type: String, val addr: String,
    val delay: Int, val ping: Int, val error: String, val link: String,
)

data class SubItem(val id: String, val name: String, val url: String, val count: Int, val updated: Long, val error: String)

data class ServerList(val servers: List<ServerItem>, val subs: List<SubItem>, val selected: String)

/** The Go side keeps the data; these helpers turn its JSON into Kotlin. */
object Store {
    fun servers(): ServerList {
        val o = JSONObject(Mobile.servers())
        val servers = o.optJSONArray("servers").objects().map {
            ServerItem(
                it.optString("id"), it.optString("sub"), it.optString("name"), it.optString("type"),
                it.optString("addr"), it.optInt("delay"), it.optInt("ping"), it.optString("error"), it.optString("link"),
            )
        }
        val subs = o.optJSONArray("subs").objects().map {
            SubItem(it.optString("id"), it.optString("name"), it.optString("url"), it.optInt("count"), it.optLong("updated"), it.optString("error"))
        }
        return ServerList(servers, subs, o.optString("selected", "auto"))
    }

    fun settings(): JSONObject = JSONObject(Mobile.getSettings())

    fun saveSettings(change: (JSONObject) -> Unit) {
        val s = settings()
        change(s)
        Mobile.saveSettings(s.toString())
    }
}

private fun JSONArray?.objects(): List<JSONObject> =
    if (this == null) emptyList() else (0 until length()).map { getJSONObject(it) }

fun JSONArray?.strings(): List<String> =
    if (this == null) emptyList() else (0 until length()).map { getString(it) }
