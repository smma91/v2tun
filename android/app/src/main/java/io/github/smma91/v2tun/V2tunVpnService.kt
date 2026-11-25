package io.github.smma91.v2tun

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.ProxyInfo
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import io.github.smma91.v2tun.libbox.BridgeOptions
import io.github.smma91.v2tun.libbox.BridgeSession
import io.github.smma91.v2tun.libbox.CommandServer
import io.github.smma91.v2tun.libbox.CommandServerHandler
import io.github.smma91.v2tun.libbox.ConnectionOwner
import io.github.smma91.v2tun.libbox.InterfaceUpdateListener
import io.github.smma91.v2tun.libbox.Libbox
import io.github.smma91.v2tun.libbox.LocalDNSTransport
import io.github.smma91.v2tun.libbox.NeighborUpdateListener
import io.github.smma91.v2tun.libbox.NetworkInterfaceIterator
import io.github.smma91.v2tun.libbox.Notification as BoxNotification
import io.github.smma91.v2tun.libbox.OverrideOptions
import io.github.smma91.v2tun.libbox.PlatformInterface
import io.github.smma91.v2tun.libbox.PlatformUser
import io.github.smma91.v2tun.libbox.ShellSession
import io.github.smma91.v2tun.libbox.StringIterator
import io.github.smma91.v2tun.libbox.SystemProxyStatus
import io.github.smma91.v2tun.libbox.TunOptions
import io.github.smma91.v2tun.libbox.WIFIState
import io.github.smma91.v2tun.mobile.Mobile
import java.net.NetworkInterface as JavaInterface
import io.github.smma91.v2tun.libbox.NetworkInterface as BoxInterface
import kotlin.concurrent.thread

/**
 * The tunnel. sing-box (libbox) runs inside this VpnService: v2tun builds the
 * configuration (Mobile.buildConfig), this class gives sing-box what it needs
 * from Android: the tunnel interface, protected sockets for its own
 * connections, and the current network.
 */
class V2tunVpnService : VpnService(), PlatformInterface, CommandServerHandler {

    companion object {
        const val ACTION_START = "io.github.smma91.v2tun.START"
        const val ACTION_STOP = "io.github.smma91.v2tun.STOP"
        private const val CHANNEL = "vpn"
        private const val NOTIFICATION_ID = 1
        private const val TAG = "v2tun"
    }

    private var server: CommandServer? = null
    private var tunFd: ParcelFileDescriptor? = null
    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }
    private var networkCallback: ConnectivityManager.NetworkCallback? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> stopTunnel()
            else -> startTunnel() // ACTION_START, or "Always-on VPN" started by Android
        }
        return START_NOT_STICKY
    }

    private fun startTunnel() {
        showNotification()
        if (server != null) return
        Vpn.set(Status.STARTING)
        thread(name = "v2tun-start") {
            try {
                val config = Mobile.buildConfig()
                val s = Libbox.newCommandServer(this, this)
                s.start()
                server = s
                s.startOrReloadService(config, OverrideOptions())
                Vpn.set(Status.STARTED)
                showNotification()
            } catch (e: Throwable) {
                Log.e(TAG, "start", e)
                cleanup()
                Vpn.set(Status.STOPPED, e.message ?: e.toString())
                stopForegroundCompat()
                stopSelf()
            }
        }
    }

    private fun stopTunnel() {
        Vpn.set(Status.STOPPING)
        thread(name = "v2tun-stop") {
            cleanup()
            Vpn.set(Status.STOPPED)
            stopForegroundCompat()
            stopSelf()
        }
    }

    private fun cleanup() {
        runCatching { server?.closeService() }
        runCatching { server?.close() }
        server = null
        runCatching { tunFd?.close() }
        tunFd = null
        networkCallback?.let { runCatching { connectivity.unregisterNetworkCallback(it) } }
        networkCallback = null
    }

    override fun onRevoke() = stopTunnel() // another VPN app took over

    override fun onDestroy() {
        cleanup()
        if (Vpn.status.value != Status.STOPPED) Vpn.set(Status.STOPPED)
        super.onDestroy()
    }

    // ---- notification (required for a foreground service) ----

    private fun showNotification() {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= 26) {
            nm.createNotificationChannel(NotificationChannel(CHANNEL, "VPN", NotificationManager.IMPORTANCE_LOW))
        }
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, V2tunVpnService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE
        )
        val connected = Vpn.status.value == Status.STARTED
        val builder = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(this, CHANNEL) else Notification.Builder(this)
        val n = builder
            .setSmallIcon(R.drawable.ic_tile)
            .setContentTitle("v2tun")
            .setContentText(if (connected) tr("Connected") else tr("Connecting…"))
            .setContentIntent(open)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, tr("Disconnect"), stop).build())
            .build()
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION_ID, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIFICATION_ID, n)
        }
    }

    private fun stopForegroundCompat() {
        if (Build.VERSION.SDK_INT >= 24) stopForeground(STOP_FOREGROUND_REMOVE) else @Suppress("DEPRECATION") stopForeground(true)
    }

    // ---- PlatformInterface: what sing-box needs from Android ----

    override fun openTun(options: TunOptions): Int {
        if (prepare(this) != null) error("no VPN permission")
        val b = Builder().setSession("v2tun").setMtu(options.mtu)
        if (Build.VERSION.SDK_INT >= 29) b.setMetered(false)
        options.inet4Address.let { while (it.hasNext()) it.next().let { a -> b.addAddress(a.address(), a.prefix()) } }
        options.inet6Address.let { while (it.hasNext()) it.next().let { a -> b.addAddress(a.address(), a.prefix()) } }
        if (options.autoRoute) {
            options.dnsServerAddress.let { while (it.hasNext()) b.addDnsServer(it.next()) }
            val r4 = options.inet4RouteAddress
            if (r4.hasNext()) while (r4.hasNext()) r4.next().let { b.addRoute(it.address(), it.prefix()) }
            else b.addRoute("0.0.0.0", 0)
            val r6 = options.inet6RouteAddress
            if (r6.hasNext()) while (r6.hasNext()) r6.next().let { b.addRoute(it.address(), it.prefix()) }
            else b.addRoute("::", 0)
            if (Build.VERSION.SDK_INT >= 33) {
                options.inet4RouteExcludeAddress.let {
                    while (it.hasNext()) it.next().let { p -> b.excludeRoute(android.net.IpPrefix(java.net.InetAddress.getByName(p.address()), p.prefix())) }
                }
                options.inet6RouteExcludeAddress.let {
                    while (it.hasNext()) it.next().let { p -> b.excludeRoute(android.net.IpPrefix(java.net.InetAddress.getByName(p.address()), p.prefix())) }
                }
            }
            // Split tunneling by app (Android does it itself).
            val include = options.includePackage
            if (include.hasNext()) {
                while (include.hasNext()) runCatching { b.addAllowedApplication(include.next()) }
            } else {
                val exclude = options.excludePackage
                while (exclude.hasNext()) runCatching { b.addDisallowedApplication(exclude.next()) }
            }
        }
        if (options.isHTTPProxyEnabled && Build.VERSION.SDK_INT >= 29) {
            b.setHttpProxy(ProxyInfo.buildDirectProxy(options.httpProxyServer, options.httpProxyServerPort))
        }
        val fd = b.establish() ?: error("the VPN was not allowed or was revoked")
        tunFd = fd
        return fd.fd
    }

    // The core's own connections must not go into the tunnel.
    override fun usePlatformAutoDetectInterfaceControl() = true
    override fun autoDetectInterfaceControl(fd: Int) {
        protect(fd)
    }

    override fun startDefaultInterfaceMonitor(listener: InterfaceUpdateListener) {
        val report = { network: Network? ->
            val lp = network?.let { connectivity.getLinkProperties(it) }
            val name = lp?.interfaceName
            if (name == null) {
                listener.updateDefaultInterface("", -1, false, false)
            } else {
                val index = runCatching { JavaInterface.getByName(name)?.index ?: -1 }.getOrDefault(-1)
                val caps = network.let { connectivity.getNetworkCapabilities(it) }
                val expensive = caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) == false
                listener.updateDefaultInterface(name, index, expensive, false)
            }
        }
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) = report(network)
            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) = report(network)
            override fun onLost(network: Network) = report(connectivity.activeNetwork.takeIf { it != network })
        }
        networkCallback = cb
        if (Build.VERSION.SDK_INT >= 24) connectivity.registerDefaultNetworkCallback(cb)
        report(connectivity.activeNetwork)
    }

    override fun closeDefaultInterfaceMonitor(listener: InterfaceUpdateListener) {
        networkCallback?.let { runCatching { connectivity.unregisterNetworkCallback(it) } }
        networkCallback = null
    }

    override fun getInterfaces(): NetworkInterfaceIterator {
        val list = mutableListOf<BoxInterface>()
        for (network in connectivity.allNetworks) {
            val lp = connectivity.getLinkProperties(network) ?: continue
            val caps = connectivity.getNetworkCapabilities(network) ?: continue
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) continue
            val name = lp.interfaceName ?: continue
            val ji = runCatching { JavaInterface.getByName(name) }.getOrNull() ?: continue
            val i = BoxInterface()
            i.name = name
            i.index = ji.index
            i.mtu = runCatching { ji.mtu }.getOrDefault(1500)
            i.addresses = Strings(lp.linkAddresses.map { "${it.address.hostAddress}/${it.prefixLength}" })
            i.dnsServer = Strings(lp.dnsServers.mapNotNull { it.hostAddress })
            i.type = when {
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> Libbox.InterfaceTypeWIFI
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> Libbox.InterfaceTypeCellular
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> Libbox.InterfaceTypeEthernet
                else -> Libbox.InterfaceTypeOther
            }
            i.metered = !caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED)
            // Unix interface flags, as Go's net package uses them.
            var flags = 0
            if (ji.isUp) flags = flags or 1 or 64 // up, running
            if (ji.isLoopback) flags = flags or 4
            if (ji.isPointToPoint) flags = flags or 8
            if (ji.supportsMulticast()) flags = flags or 16
            if (!ji.isLoopback && !ji.isPointToPoint) flags = flags or 2 // broadcast
            i.flags = flags
            list += i
        }
        return object : NetworkInterfaceIterator {
            private val it = list.iterator()
            override fun hasNext() = it.hasNext()
            override fun next(): BoxInterface = it.next()
        }
    }

    override fun findConnectionOwner(
        ipProtocol: Int, sourceAddress: String, sourcePort: Int, destinationAddress: String, destinationPort: Int
    ): ConnectionOwner {
        if (Build.VERSION.SDK_INT < 29) error("not supported")
        val uid = connectivity.getConnectionOwnerUid(
            ipProtocol,
            java.net.InetSocketAddress(sourceAddress, sourcePort),
            java.net.InetSocketAddress(destinationAddress, destinationPort)
        )
        if (uid == android.os.Process.INVALID_UID) error("unknown")
        val o = ConnectionOwner()
        o.userId = uid
        o.setAndroidPackageNames(Strings(packageManager.getPackagesForUid(uid)?.toList() ?: emptyList()))
        return o
    }

    override fun localDNSTransport(): LocalDNSTransport? = null
    override fun underNetworkExtension() = false
    override fun includeAllNetworks() = false
    override fun readWIFIState(): WIFIState? = null
    override fun clearDNSCache() {}
    override fun sendNotification(notification: BoxNotification) {}
    override fun cancelNotification(identifier: String, typeID: Int) {}
    override fun startNeighborMonitor(listener: NeighborUpdateListener) {}
    override fun closeNeighborMonitor(listener: NeighborUpdateListener) {}
    override fun registerMyInterface(name: String) {}
    override fun usePlatformShell() = false
    override fun checkPlatformShell() = error("not supported")
    override fun openShellSession(user: PlatformUser, command: String, environ: StringIterator, term: String, rows: Int, cols: Int): ShellSession =
        error("not supported")
    override fun lookupUser(username: String): PlatformUser = error("not supported")
    override fun lookupSFTPServer(): String = error("not supported")
    override fun readSystemSSHHostKey(): String = error("not supported")
    override fun tailscaleHostname() = ""
    override fun usePlatformBridge() = false
    override fun createBridge(options: BridgeOptions): BridgeSession = error("not supported")

    // ---- CommandServerHandler ----

    override fun serviceStop() = stopTunnel()
    override fun serviceReload() {
        val s = server ?: return
        thread { runCatching { s.startOrReloadService(Mobile.buildConfig(), OverrideOptions()) } }
    }
    override fun getSystemProxyStatus(): SystemProxyStatus = SystemProxyStatus()
    override fun setSystemProxyEnabled(enabled: Boolean) {}
    override fun triggerNativeCrash() {}
    override fun writeDebugMessage(message: String) {
        Log.d(TAG, message)
    }
    override fun connectSSHAgent(): Int = error("not supported")
}

/** A Kotlin list as sing-box's string iterator. */
class Strings(list: List<String>) : StringIterator {
    private val items = list
    private var i = 0
    override fun hasNext() = i < items.size
    override fun len() = items.size
    override fun next(): String = items[i++]
}

@Suppress("unused")
private fun PackageManager.hasLeanback() = hasSystemFeature(PackageManager.FEATURE_LEANBACK)
