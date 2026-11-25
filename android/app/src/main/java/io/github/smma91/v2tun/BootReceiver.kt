package io.github.smma91.v2tun

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.VpnService

/** "Auto-connect": connect when the device starts (if the VPN was allowed before). */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (!Store.settings().optBoolean("autoConnect")) return
        if (VpnService.prepare(context) != null) return
        Vpn.start(context)
    }
}
