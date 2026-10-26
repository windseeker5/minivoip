import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Signing passwords live in local.properties (never in git).
val localProperties = Properties().apply {
    val file = rootProject.file("local.properties")
    if (file.exists()) file.inputStream().use { load(it) }
}

android {
    namespace = "com.kdresdell.minivoip"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.kdresdell.minivoip"
        minSdk = 26
        targetSdk = 35
        versionCode = 3
        versionName = "0.3"
        // The minivoip server this app is the phone for.
        buildConfigField("String", "SERVER", "\"${localProperties.getProperty("SERVER", "https://voip.dresdell.com")}\"")
    }

    signingConfigs {
        create("release") {
            localProperties.getProperty("RELEASE_STORE_FILE")?.let {
                storeFile = rootProject.file(it)
                storePassword = localProperties.getProperty("RELEASE_STORE_PASSWORD")
                keyAlias = localProperties.getProperty("RELEASE_KEY_ALIAS")
                keyPassword = localProperties.getProperty("RELEASE_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
    }

    // Lint crashes under this machine's new JDK (see kdtv); sideloaded app.
    lint {
        checkReleaseBuilds = false
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        buildConfig = true
    }
}

dependencies {
    implementation("com.google.firebase:firebase-messaging:24.1.0")
}
