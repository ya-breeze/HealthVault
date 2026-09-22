plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

val deliveryVersionCodeText = providers.gradleProperty("androidDeliveryVersionCode").orNull
val deliveryVersionName = providers.gradleProperty("androidDeliveryVersionName").orNull
val deliveryRequested = deliveryVersionCodeText != null || deliveryVersionName != null

val deliveryVersionCode = if (deliveryRequested) {
    require(!deliveryVersionCodeText.isNullOrBlank() && !deliveryVersionName.isNullOrBlank()) {
        "androidDeliveryVersionCode and androidDeliveryVersionName must be provided together"
    }
    deliveryVersionCodeText.toIntOrNull()?.takeIf { it > 0 }
        ?: error("androidDeliveryVersionCode must be a positive integer")
} else {
    null
}

fun requiredDeliveryEnvironment(name: String): String =
    providers.environmentVariable(name).orNull?.takeIf { it.isNotBlank() }
        ?: error("$name is required for an Android delivery build")

val deliveryKeystoreFile = if (deliveryRequested) {
    file(requiredDeliveryEnvironment("ANDROID_DELIVERY_KEYSTORE_FILE")).also {
        require(it.isFile) { "ANDROID_DELIVERY_KEYSTORE_FILE must name an existing file" }
    }
} else {
    null
}
val deliveryKeystorePassword =
    if (deliveryRequested) requiredDeliveryEnvironment("ANDROID_DELIVERY_KEYSTORE_PASSWORD") else null
val deliveryKeyAlias =
    if (deliveryRequested) requiredDeliveryEnvironment("ANDROID_DELIVERY_KEY_ALIAS") else null
val deliveryKeyPassword =
    if (deliveryRequested) requiredDeliveryEnvironment("ANDROID_DELIVERY_KEY_PASSWORD") else null

android {
    namespace = "net.ikoro.healthvault"
    compileSdk = 34

    defaultConfig {
        applicationId = "net.ikoro.healthvault"
        minSdk = 26 // Glance's SizeMode.Responsive and the widget receiver APIs used here require 26+.
        targetSdk = 34
        versionCode = deliveryVersionCode ?: 1
        versionName = deliveryVersionName ?: "1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        if (deliveryRequested) {
            create("deliveryRelease") {
                storeFile = deliveryKeystoreFile
                storePassword = deliveryKeystorePassword
                keyAlias = deliveryKeyAlias
                keyPassword = deliveryKeyPassword
            }
        }
    }

    buildTypes {
        debug {
            // debug/res/xml/network_security_config.xml (Task 1) permits
            // cleartext only in this build type, so a LAN stack like
            // http://192.168.1.54:8892 stays reachable in development
            // while release keeps the platform's HTTPS-only default.
            isDebuggable = true
        }
        release {
            isMinifyEnabled = false
            if (deliveryRequested) {
                signingConfig = signingConfigs.getByName("deliveryRelease")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    sourceSets {
        getByName("main").kotlin.srcDirs("src/main/kotlin")
        getByName("test").kotlin.srcDirs("src/test/kotlin")
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)

    implementation(libs.androidx.glance.appwidget)
    implementation(libs.androidx.glance.material3)

    implementation(libs.androidx.work.runtime.ktx)
    implementation(libs.androidx.browser)
    implementation(libs.androidx.security.crypto)
    implementation(libs.androidx.appcompat) // AppCompatDelegate.setApplicationLocales: per-app language back-ported below API 33.

    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    debugImplementation(libs.androidx.compose.ui.tooling)

    testImplementation(libs.junit)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.kotlinx.coroutines.test)
}
