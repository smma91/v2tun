<div dir="rtl">

<h1 align="center">v2tun</h1>

<p align="center">
  <b>بنقرة واحدة تمر كل تطبيقات حاسوبك عبر الوكيل، مع مفتاح إيقاف حقيقي.</b><br>
  نفق شامل لنظامَي <b>Windows</b> و<b>Linux</b>، بنواة مدمجة أو عبر v2rayN الذي تستخدمه.
</p>

<p align="center">
  <a href="https://github.com/smma91/v2tun/releases/latest"><img src="https://img.shields.io/github/v/release/smma91/v2tun?label=download&color=2563eb" alt="Latest release"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-0f172a" alt="Windows | Linux">
  <a href="https://github.com/smma91/v2tun/releases"><img src="https://img.shields.io/github/downloads/smma91/v2tun/total?color=16a34a" alt="Downloads"></a>
  <a href="../LICENSE"><img src="https://img.shields.io/github/license/smma91/v2tun?color=64748b" alt="License"></a>
</p>

<p align="center">
  <a href="../README.md">English</a> | <a href="README.fa.md">فارسی</a> | <b>العربية</b> | <a href="README.zh.md">中文</a>
</p>

<p align="center">
  <img src="screenshots/connect.png" width="260" alt="Connect">
  <img src="screenshots/servers.png" width="260" alt="Servers">
  <img src="screenshots/settings.png" width="260" alt="Settings">
</p>

## لماذا v2tun؟

| | |
|---|---|
| 🌐 **كل التطبيقات، لا المتصفح فقط** | الألعاب والمراسلة والتحديثات والطرفية تمر كلها عبر النفق، دون إعداد وكيل لكل تطبيق. |
| 🛡️ **لا شيء يتسرب** | مفتاح إيقاف، وحماية من تسرب IPv6 وDNS، وDNS عن بُعد: اختبارات التسرب لا تُظهر إلا خادمك. |
| 📶 **شاركه عبر Wi-Fi** | حوّل حاسوبك إلى نقطة اتصال؛ الهواتف والحواسيب المتصلة بها تمر عبر النفق أيضًا. |
| ⚡ **كل البروتوكولات الحديثة** | VLESS Reality / XHTTP وVMess وTrojan وShadowsocks وHysteria2 وTUIC وWireGuard وNaiveProxy وOpenVPN وAnyConnect وغيرها. |
| 🚀 **دائمًا أسرع خادم** | Ping وزمن الاستجابة الحقيقي واختبار السرعة، ووضع «تلقائي» يبدّل الخادم دون قطع التنزيلات. |
| 🔌 **يعمل مع v2rayN** | إن كنت تستخدم v2rayN فإن v2tun يجده ويمرر وكيله للنظام كله. |
| 📱 **على الهاتف والتلفاز أيضًا** | تطبيق أندرويد (هواتف وأجهزة لوحية وAndroid TV) بالخوادم نفسها، مع الإضافة عبر QR والمشاركة. |
| 🧩 **بسيط ومجاني** | برنامج صغير بواجهة داكنة أنيقة بأربع لغات، مفتوح المصدر (GPL-3.0). |

## الميزات

### 🛡️ الخصوصية والحماية
- **نفق TUN شامل** لكل التطبيقات (Wintun في Windows و`/dev/net/tun` في Linux).
- **مفتاح إيقاف:** إذا انقطع النفق أو الوكيل يبقى الإنترنت محجوبًا بدل الرجوع إلى الاتصال الحقيقي.
- **حماية من تسرب IPv6 وDNS** حتى عند إيقاف مفتاح الإيقاف.
- **DNS عن بُعد (FakeIP):** لا تغادر طلبات DNS حاسوبك، ويحلّ الخادم الأسماء في بلد الخروج.
- **اختبار تسرب مدمج** (IP وWebRTC) وعرض IP الخروج مع البلد والمزوّد.
- **مطابقة المنطقة الزمنية** مع بلد الخادم (اختياري).
- **أمان عند الإغلاق المفاجئ:** عملية مراقبة تعيد الشبكة والمنطقة الزمنية وإعداد الوكيل.

### ⚡ الخوادم والبروتوكولات
- **نواة [sing-box](https://github.com/SagerNet/sing-box) مدمجة:** VLESS (Reality، وXHTTP عبر Xray)، VMess، Trojan، Shadowsocks، Hysteria / Hysteria2، TUIC، WireGuard، NaiveProxy، SOCKS، HTTP، OpenVPN وAnyConnect / OpenConnect / Fortinet / GlobalProtect.
- **اشتراكات** بصيغ base64 وsing-box وClash مع تحديث تلقائي.
- **Ping وزمن الاستجابة الحقيقي واختبار السرعة** لخادم واحد أو للجميع.
- **تلقائي (الأسرع)** مع تبديل الخادم دون قطع الاتصالات المفتوحة.
- **وضع v2rayN:** استخدم خوادم v2rayN التي لديك.

### 📶 المشاركة
- **نقطة اتصال Wi-Fi:** الهواتف والأجهزة اللوحية والحواسيب المتصلة بها تستخدم النفق بالحماية نفسها؛ إذا انقطع النفق ينقطع عنها الإنترنت ولا يتسرب شيء.
- **وكيل محلي:** SOCKS5 وHTTP على منفذ واحد، مع إمكانية الوصول من الشبكة المحلية.
- **وضع «وكيل فقط»:** دون نفق، الوكيل المحلي فقط كوكيل للنظام.

### 🧭 التحكم
- **تقسيم النفق** حسب التطبيق وحسب النطاق / نطاق IP، بوضعَي «الكل ما عدا هذه» و«هذه فقط».
- **تعامل ذكي مع QUIC:** عندما لا ينقل الخادم UDP تتحول التطبيقات إلى TCP فورًا دون انتظار.

### 📱 أندرويد وAndroid TV
- البروتوكولات والاشتراكات نفسها؛ أضف الخوادم من الحافظة أو **بمسح رمز QR** (بالكاميرا أو من صورة) أو بمشاركة رابط مع v2tun.
- **مشاركة أي خادم أو اشتراك** كرمز QR أو رابط.
- سرعة مباشرة، وعنوان IP الخروج مع البلد، وPing للخوادم، ومربع الإعدادات السريعة، والاتصال التلقائي عند التشغيل.
- **وكيل محلي** للأجهزة الأخرى على شبكة Wi-Fi نفسها أو على نقطة اتصال الهاتف.
- Android TV: مصمم للتحكم بجهاز التحكم عن بُعد، مع لافتة على الشاشة الرئيسية للتلفاز.
- موفّر للبطارية: الإحصاءات المباشرة فقط عندما يكون التطبيق على الشاشة، والنواة تتوقف في وضع Doze.
- أندرويد 7.0 أو أحدث.

### 🖥️ الواجهة
- أيقونة بجانب الساعة، تشغيل عند تسجيل الدخول (دون كلمة مرور أو UAC)، اتصال تلقائي.
- واجهة **بالإنجليزية والفارسية والعربية والصينية**.
- Windows 10/11 وLinux (Ubuntu وDebian وMint وFedora وAppImage).

## التنزيل

نزّل أحدث إصدار من **[Releases](https://github.com/smma91/v2tun/releases/latest)**. طريقة التثبيت لكل نظام موضحة أدناه.

## متطلبات Windows

- Windows 10 أو 11، ‏64 بت (x64).
- صلاحيات المسؤول (Administrator) لمحول النفق والمسارات والجدار الناري.
- [WebView2](https://developer.microsoft.com/microsoft-edge/webview2/) (موجود في Windows 11 وفي Windows 10 المحدَّث).

## التثبيت على Windows

1. نزّل `v2tun-<version>-windows-amd64.zip` من [Releases](https://github.com/smma91/v2tun/releases) وتحقق منه عبر `SHA256SUMS.txt`.
2. فك الضغط في أي مجلد وشغّل `v2tun.exe` (يطلب صلاحيات المسؤول).
3. أضف خوادم من تبويب **الخوادم** (الصق رابطًا أو اشتراكًا)، أو اختر **v2rayN** مصدرًا للوكيل أثناء تشغيله.
4. اضغط زر التشغيل.

تُحفظ الإعدادات والخوادم والسجل في `C:\ProgramData\v2tun` (للمسؤولين فقط).

> **تحذير مكافح الفيروسات.** يغيّر v2tun الجدار الناري والمسارات وDNS وملف hosts ويحمّل برنامج تشغيل شبكة. قد تُصنَّف النسخ غير الموقّعة من هذه الأدوات خطأً كبرامج ضارة. يمكنك دائمًا بناء الملف التنفيذي بنفسك من هذا المصدر.

## Linux

**المتطلبات:** توزيعة 64 بت (x86_64) مع WebKitGTK 4.1، مثل Ubuntu 22.04 أو أحدث، Debian 12، Linux Mint 21 أو Fedora 37 أو أحدث.

نزّل الملف المناسب من [Releases](https://github.com/smma91/v2tun/releases):

| الحزمة | التثبيت |
|---|---|
| `.deb` (Ubuntu, Debian, Mint) | `sudo apt install ./v2tun_<version>_amd64.deb` |
| `.rpm` (Fedora) | `sudo dnf install ./v2tun-<version>-1.x86_64.rpm` |
| `.AppImage` | `chmod +x v2tun-*.AppImage` |
| `.tar.gz` | `sudo ./install.sh` |

ثم شغّل **v2tun** من قائمة التطبيقات.

- لا يعمل v2tun بصلاحيات root؛ تفتح النافذة بمستخدمك، ويحصل البرنامج فقط على صلاحيتَي الشبكة اللازمتين (`cap_net_admin` و`cap_net_raw`). تمنحهما الحزم و`install.sh`؛ وإن لم تتوفرا (مثل AppImage) يظهر زر **منح الإذن** الذي يطلب كلمة المرور مرة واحدة، ويثبّت AppImage نفسه حينها في `/opt/v2tun`.
- تحتاج أيقونة شريط النظام إلى سطح مكتب يدعمها (KDE وXfce وCinnamon وUbuntu؛ في GNOME العادي ثبّت إضافة AppIndicator). بدونها يُصغَّر الإطار عند إغلاقه.
- تُحفظ الإعدادات والخوادم في `~/.config/v2tun`.

## أندرويد وAndroid TV

**المتطلبات:** أندرويد 7.0 أو أحدث (هواتف، أجهزة لوحية، Android TV).

نزّل ملف APK من [Releases](https://github.com/smma91/v2tun/releases) وتحقق منه باستخدام `SHA256SUMS.txt`:

| الملف | مناسب لـ |
|---|---|
| `v2tun-<version>-android-arm64-v8a.apk` | معظم الهواتف وأجهزة التلفاز الحديثة |
| `v2tun-<version>-android-armeabi-v7a.apk` | الهواتف القديمة ومعظم أجهزة وعصي التلفاز |
| `v2tun-<version>-android-universal.apk` | أي جهاز (حجم أكبر) |
| `v2tun-<version>-android-x86_64.apk` | المحاكيات وأجهزة x86 |

افتح الملف لتثبيته (اسمح بالتثبيت من هذا المصدر إذا طلب أندرويد ذلك). على التلفاز، انقل ملف APK بذاكرة USB أو بتطبيق مثل *Send files to TV*، أو ثبّته عبر `adb install`.

عند الاتصال لأول مرة يطلب أندرويد السماح بـ VPN. لمفتاح الإيقاف (Kill switch)، افتح **الإعدادات ← Kill switch** في v2tun (يفتح إعدادات VPN في أندرويد) وفعّل *شبكة VPN قيد التشغيل دائمًا* و*حظر الاتصالات بدون VPN*.

## البناء من المصدر

المتطلبات: Go 1.26 (يقوم الأمر `go` بتنزيل الإصدار الصحيح تلقائيًا) وPowerShell. لا حاجة لمترجم C.

<div dir="ltr">

```powershell
git clone https://github.com/smma91/v2tun.git
cd v2tun
.\build.ps1                 # or: .\build.ps1 -Version 1.0.0
```

</div>

الناتج ملف واحد `v2tun.exe` يتضمن `wintun.dll` و`libcronet.dll`.

**Linux:**

<div dir="ltr">

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
./build.sh
sudo setcap cap_net_admin,cap_net_raw+ep ./v2tun
```

</div>

الناتج `v2tun` و`libcronet.so` ويجب أن يبقيا معًا. تفاصيل الاختبارات والترجمة في [README الإنجليزي](../README.md).

## الملاحظات والمشاكل

وجدت خطأً أو لديك فكرة؟ [افتح Issue](https://github.com/smma91/v2tun/issues/new/choose). احذف عناوين الخوادم وكلمات المرور وروابط الاشتراك من السجلات قبل نشرها. لا تُقبل طلبات الدمج (Pull Requests) حاليًا.

## ☕ ادعم المشروع

v2tun مجاني ومفتوح المصدر. إن أفادك يمكنك دعوتي إلى فنجان قهوة:

| الشبكة | العنوان |
|---|---|
| Ethereum وBNB Smart Chain وPolygon وArbitrum وشبكات EVM الأخرى (USDT وUSDC وETH وBNB…) | <code dir="ltr">0x1404A760e5A30f9DD1D3AC68726673f94DC2f649</code> |

يُرجى التحقق من العنوان والشبكة قبل الإرسال. النجوم ⭐ وتقارير الأخطاء تساعد كثيرًا أيضًا.

## الترخيص

يُوزَّع v2tun بموجب [GNU General Public License v3.0](../LICENSE) لأنه يتضمن sing-box ومكتبات أخرى مرخصة بـ GPL. المكونات الخارجية وتراخيصها مذكورة في [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

يُقدَّم هذا البرنامج «كما هو» دون أي ضمان. أنت مسؤول عن استخدامه وفق قوانين بلدك وشروط مزوّد الوكيل.

</div>
