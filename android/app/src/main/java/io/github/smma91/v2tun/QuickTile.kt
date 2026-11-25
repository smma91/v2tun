package io.github.smma91.v2tun

import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/** Quick Settings tile: connect / disconnect without opening the app. */
class QuickTile : TileService() {
    private var scope: CoroutineScope? = null

    override fun onStartListening() {
        val s = CoroutineScope(Dispatchers.Main + Job())
        scope = s
        s.launch { Vpn.status.collect { update(it) } }
    }

    override fun onStopListening() {
        scope?.cancel()
        scope = null
    }

    private fun update(status: Status) {
        val tile = qsTile ?: return
        tile.state = when (status) {
            Status.STARTED -> Tile.STATE_ACTIVE
            Status.STOPPED -> Tile.STATE_INACTIVE
            else -> Tile.STATE_UNAVAILABLE
        }
        tile.label = "v2tun"
        if (Build.VERSION.SDK_INT >= 29) tile.subtitle = if (status == Status.STARTED) tr("Connected") else null
        tile.updateTile()
    }

    override fun onClick() {
        when (Vpn.status.value) {
            Status.STARTED -> Vpn.stop(this)
            Status.STOPPED -> if (VpnService.prepare(this) == null) Vpn.start(this) else openApp()
            else -> {}
        }
    }

    // The first connection needs Android's VPN permission dialog, from the app.
    @Suppress("DEPRECATION")
    private fun openApp() {
        val i = android.content.Intent(this, MainActivity::class.java).addFlags(android.content.Intent.FLAG_ACTIVITY_NEW_TASK)
        if (Build.VERSION.SDK_INT >= 34) {
            startActivityAndCollapse(android.app.PendingIntent.getActivity(this, 0, i, android.app.PendingIntent.FLAG_IMMUTABLE))
        } else {
            startActivityAndCollapse(i)
        }
    }
}
