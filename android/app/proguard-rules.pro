# R8 (shrinking the app code). The Go library is called through JNI by class
# and method names, so its Java side and everything that implements its
# interfaces must keep their names.

# gomobile runtime and the bound Go packages (sing-box's libbox, v2tun's mobile).
-keep class go.** { *; }
-keep class io.github.smma91.v2tun.libbox.** { *; }
-keep class io.github.smma91.v2tun.mobile.** { *; }

# Kotlin classes Go calls back into (PlatformInterface, CommandServerHandler,
# CommandClientHandler, iterators, Protector, ...).
-keep class * implements io.github.smma91.v2tun.libbox.** { *; }
-keep class * implements io.github.smma91.v2tun.mobile.** { *; }

# Readable stack traces in crash reports.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile
