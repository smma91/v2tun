plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Version from the release tag (CI sets V2TUN_VERSION), e.g. "0.12.0".
val appVersion = System.getenv("V2TUN_VERSION")?.takeIf { it.isNotBlank() } ?: "0.12.0-dev"
val versionParts = appVersion.substringBefore('-').split('.').map { it.toIntOrNull() ?: 0 }
val appVersionCode = versionParts.getOrElse(0) { 0 } * 10000 + versionParts.getOrElse(1) { 0 } * 100 + versionParts.getOrElse(2) { 0 }

android {
    namespace = "io.github.smma91.v2tun"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.smma91.v2tun"
        minSdk = 24
        targetSdk = 35
        versionCode = maxOf(appVersionCode, 1)
        versionName = appVersion
    }

    // Release signing comes from the environment (GitHub secrets); without it
    // the release build is signed with the debug key (for testing only).
    signingConfigs {
        create("release") {
            val ks = System.getenv("V2TUN_KEYSTORE")
            if (ks != null && file(ks).exists()) {
                storeFile = file(ks)
                storePassword = System.getenv("V2TUN_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("V2TUN_KEY_ALIAS")
                keyPassword = System.getenv("V2TUN_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = if (System.getenv("V2TUN_KEYSTORE") != null) signingConfigs.getByName("release")
            else signingConfigs.getByName("debug")
        }
    }

    // One APK per processor type (smaller downloads), plus a universal one.
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = true
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures { compose = true }

    packaging {
        jniLibs { useLegacyPackaging = true } // the Go library is large; extracted once at install
    }

    // The translations are shared with the desktop app.
    sourceSets["main"].assets.srcDir(layout.buildDirectory.dir("generated/i18n"))
}

val copyI18n by tasks.registering(Copy::class) {
    from(rootProject.file("../cmd/v2tun/frontend/i18n.json"))
    into(layout.buildDirectory.dir("generated/i18n"))
}
tasks.named("preBuild") { dependsOn(copyI18n) }

dependencies {
    implementation(files("libs/libv2tun.aar"))

    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material:material-icons-core")
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.core:core-ktx:1.15.0")
}
