package io.github.smma91.v2tun

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.PowerManager
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
import io.github.smma91.v2tun.libbox.CommandClient
import io.github.smma91.v2tun.libbox.CommandClientHandler
import io.github.smma91.v2tun.libbox.CommandClientOptions
import io.github.smma91.v2tun.libbox.ConnectionEvents
import io.github.smma91.v2tun.libbox.CommandServer
import io.github.smma91.v2tun.libbox.LogIterator
import io.github.smma91.v2tun.libbox.OutboundGroupItemIterator
import io.github.smma91.v2tun.libbox.OutboundGroupIterator
import io.github.smma91.v2tun.libbox.StatusMessage
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

        /** The running service (its protect() keeps sockets out of the tunnel). */
        @Volatile var instance: V2tunVpnService? = null
            private set
    }

    override fun onCreate() {
        super.onCreate()
        instance = this
    }

    private var server: CommandServer? = null
    private var tunFd: ParcelFileDescriptor? = null
    private val connectivity by lazy { getSystemService(CONNECTIVITY_SERVICE) as ConnectivityManager }
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private var statusClient: CommandClient? = null
    @Volatile private var logClient: CommandClient? = null
    private var lastStatusAt = 0L

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
                listenForIdle()
                updateWatching()
                lookUpExit()
            } catch (e: Throwable) {
                Log.e(TAG, "start", e)
                CoreLog.add(listOf(CoreLog.LEVEL_ERROR to (e.message ?: e.toString())))
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
        runCatching { statusClient?.disconnect() }
        statusClient = null
        runCatching { logClient?.disconnect() }
        logClient = null
        if (idleListening) { runCatching { unregisterReceiver(idleReceiver) }; idleListening = false }
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
        instance = null
        Vpn.online.value = true
        cleanup()
        if (Vpn.status.value != Status.STOPPED) Vpn.set(Status.STOPPED)
        super.onDestroy()
    }

    // ---- battery ----

    // Deep sleep (Doze): the core pauses its timers and keep-alives, and wakes
    // up when the device does (as sing-box's own app does).
    private var idleListening = false
    private val idleReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            val s = server ?: return
            if (Build.VERSION.SDK_INT < 23) return
            val idle = (getSystemService(POWER_SERVICE) as PowerManager).isDeviceIdleMode
            runCatching { if (idle) s.pause() else s.wake() }
        }
    }

    private fun listenForIdle() {
        if (Build.VERSION.SDK_INT < 23 || idleListening) return
        registerReceiver(idleReceiver, IntentFilter(PowerManager.ACTION_DEVICE_IDLE_MODE_CHANGED))
        idleListening = true
    }

    /** Live speed is only collected while the app is on screen, the log only while the Log screen is open. */
    @Synchronized
    fun updateWatching() {
        val want = Vpn.status.value == Status.STARTED && Vpn.uiVisible
        if (want && statusClient == null) {
            lastStatusAt = 0L
            watchTraffic()
        } else if (!want && statusClient != null) {
            val c = statusClient
            statusClient = null
            thread { runCatching { c?.disconnect() } }
        }
        val wantLog = want && CoreLog.visible
        if (wantLog && logClient == null) {
            watchLog()
        } else if (!wantLog && logClient != null) {
            val c = logClient
            logClient = null
            thread { runCatching { c?.disconnect() } }
        }
    }

    /** Asks the core to forget its saved log lines (the Log screen's "Clear"). */
    fun clearLog() {
        val c = logClient ?: return
        thread { runCatching { c.clearLogs() } }
    }

    // ---- live traffic and exit IP (shown on the main screen) ----

    private fun watchTraffic() {
        val o = CommandClientOptions()
        o.addCommand(Libbox.CommandStatus)
        // The core's log in logcat: debug builds only.
        if (applicationInfo.flags and android.content.pm.ApplicationInfo.FLAG_DEBUGGABLE != 0) o.addCommand(Libbox.CommandLog)
        o.statusInterval = 1_000_000_000L // 1 s
        val c = Libbox.newCommandClient(object : CommandClientHandler {
            override fun writeStatus(message: StatusMessage) {
                if (Vpn.status.value != Status.STARTED) return
                // Speed from the change in the totals (the core only reports totals here).
                val now = System.nanoTime()
                val prev = Vpn.traffic.value
                val secs = if (lastStatusAt == 0L) 0.0 else (now - lastStatusAt) / 1e9
                lastStatusAt = now
                val up = if (prev != null && secs > 0.2) ((message.uplinkTotal - prev.upTotal).coerceAtLeast(0) / secs).toLong() else 0L
                val down = if (prev != null && secs > 0.2) ((message.downlinkTotal - prev.downTotal).coerceAtLeast(0) / secs).toLong() else 0L
                Vpn.traffic.value = Traffic(up, down, message.uplinkTotal, message.downlinkTotal)
            }
            override fun connected() {}
            override fun disconnected(message: String?) {}
            override fun clearLogs() {}
            override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
            override fun setDefaultLogLevel(level: Int) {}
            override fun updateClashMode(newMode: String?) {}
            override fun writeConnectionEvents(events: ConnectionEvents?) {}
            override fun writeGroups(message: OutboundGroupIterator?) {}
            override fun writeLogs(messageList: LogIterator?) {
                val it = messageList ?: return
                while (it.hasNext()) Log.i("v2tun-core", it.next().message)
            }
            override fun writeOutbounds(message: OutboundGroupItemIterator?) {}
        }, o)
        statusClient = c
        thread(name = "v2tun-status") { runCatching { c.connect() }.onFailure { Log.w(TAG, "status", it) } }
    }

    /** The core's log for the Log screen (it first sends the lines it kept, after a reset). */
    private fun watchLog() {
        val o = CommandClientOptions()
        o.addCommand(Libbox.CommandLog)
        var self: CommandClient? = null
        val c = Libbox.newCommandClient(object : CommandClientHandler {
            override fun clearLogs() {
                if (logClient === self) CoreLog.reset()
            }
            override fun writeLogs(messageList: LogIterator?) {
                val it = messageList ?: return
                val entries = ArrayList<Pair<Int, String>>()
                while (it.hasNext()) it.next().let { e -> entries += e.level to (e.message ?: "") }
                if (logClient === self) CoreLog.add(entries)
            }
            override fun writeStatus(message: StatusMessage) {}
            override fun connected() {}
            override fun disconnected(message: String?) {}
            override fun initializeClashMode(modeList: StringIterator?, currentMode: String?) {}
            override fun setDefaultLogLevel(level: Int) {}
            override fun updateClashMode(newMode: String?) {}
            override fun writeConnectionEvents(events: ConnectionEvents?) {}
            override fun writeGroups(message: OutboundGroupIterator?) {}
            override fun writeOutbounds(message: OutboundGroupItemIterator?) {}
        }, o)
        self = c
        logClient = c
        thread(name = "v2tun-log") { runCatching { c.connect() }.onFailure { Log.w(TAG, "log", it) } }
    }

    private fun lookUpExit() {
        thread(name = "v2tun-exit") {
            repeat(3) { attempt ->
                if (Vpn.status.value != Status.STARTED) return@thread
                val r = runCatching { org.json.JSONObject(Mobile.exitInfo()) }.getOrNull()
                if (r != null) {
                    if (Vpn.status.value == Status.STARTED) {
                        Vpn.exit.value = Exit(r.optString("ip"), r.optString("country"), r.optString("city"), r.optString("org"))
                    }
                    return@thread
                }
                Thread.sleep(2000L * (attempt + 1))
            }
        }
    }

    // ---- notification (required for a foreground service) ----

    private fun showNotification() {
        val nm = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
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
            .addAction(@Suppress("DEPRECATION") Notification.Action.Builder(0, tr("Disconnect"), stop).build())
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
                runCatching { b.addAllowedApplication(packageName) } // v2tun's own checks (exit IP) go through the tunnel
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
        // The best real (non-VPN) network: asking for the "default" network
        // would return this VPN itself.
        var current: Network? = null
        val report: (Network?) -> Unit = { network ->
            current = network
            val name = network?.let { connectivity.getLinkProperties(it)?.interfaceName }
            if (network == null || name == null) {
                Log.i(TAG, "network: none")
                Vpn.online.value = false
                listener.updateDefaultInterface("", -1, false, false)
            } else {
                val index = runCatching { JavaInterface.getByName(name)?.index ?: -1 }.getOrDefault(-1)
                val caps = connectivity.getNetworkCapabilities(network)
                val metered = caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_METERED) == false
                Log.i(TAG, "network: $name ($index)")
                val wasOffline = !Vpn.online.value
                Vpn.online.value = true
                // Back online (or another network): the exit may have changed.
                if (Vpn.status.value == Status.STARTED && (wasOffline || Vpn.exit.value == null)) lookUpExit()
                listener.updateDefaultInterface(name, index, metered, false)
            }
            // Android shows the VPN as using this network (metered, validated...).
            if (Build.VERSION.SDK_INT >= 22) runCatching { setUnderlyingNetworks(network?.let { arrayOf(it) }) }
        }
        // Right away, so the core does not start without a network.
        report(realNetwork(null))
        val cb = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                if (network != current) report(network)
            }
            override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) {
                if (network == current || current == null) report(network)
            }
            override fun onLost(network: Network) {
                if (network == current) report(realNetwork(network))
            }
        }
        networkCallback = cb
        val request = android.net.NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_RESTRICTED)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        val handler = android.os.Handler(monitorThread.looper)
        try {
            if (Build.VERSION.SDK_INT >= 31) {
                connectivity.registerBestMatchingNetworkCallback(request, cb, handler)
            } else if (Build.VERSION.SDK_INT >= 26) {
                connectivity.requestNetwork(request, cb, handler)
            } else {
                connectivity.requestNetwork(request, cb) // callbacks on the main thread
            }
        } catch (e: SecurityException) {
            // Android 6.0 wants a special permission for requestNetwork. Only
            // listening needs none: pick the best real network on every change.
            Log.i(TAG, "network: listening only (${e.message})")
            val listen = object : ConnectivityManager.NetworkCallback() {
                private fun recheck() {
                    val best = realNetwork(null)
                    val name = best?.let { connectivity.getLinkProperties(it)?.interfaceName }
                    val currentName = current?.let { connectivity.getLinkProperties(it)?.interfaceName }
                    if (best != current || name != currentName) report(best)
                }
                override fun onAvailable(network: Network) = recheck()
                override fun onLost(network: Network) = recheck()
                override fun onLinkPropertiesChanged(network: Network, lp: LinkProperties) = recheck()
            }
            networkCallback = listen
            connectivity.registerNetworkCallback(request, listen)
        }
    }

    private val monitorThread by lazy { android.os.HandlerThread("v2tun-network").apply { start() } }

    /** A real network with internet (Wi-Fi / Ethernet before mobile data), other than [except]. */
    @Suppress("DEPRECATION")
    private fun realNetwork(except: Network?): Network? {
        val usable = connectivity.allNetworks.filter { n ->
            n != except && connectivity.getNetworkCapabilities(n)?.let { c ->
                c.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
                    c.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN) &&
                    c.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_RESTRICTED)
            } == true && connectivity.getLinkProperties(n)?.interfaceName != null
        }
        fun caps(n: Network) = connectivity.getNetworkCapabilities(n)
        return usable.firstOrNull { caps(it)?.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) == true || caps(it)?.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) == true }
            ?: usable.firstOrNull { caps(it)?.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) == true }
            ?: usable.firstOrNull()
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
            if (!caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)) continue // e.g. the operator's IMS (VoLTE) network
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
