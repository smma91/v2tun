<div dir="rtl">

# v2tun

[English](../README.md) | [فارسی](README.fa.md) | **العربية** | [中文](README.zh.md)

نفق شامل مع مفتاح إيقاف (Kill Switch) لنظامَي **Windows وLinux**. يمرّر v2tun حركة جميع التطبيقات على الحاسوب عبر وكيل (Proxy)، إما بنواته المدمجة (الخوادم والاشتراكات داخل التطبيق) أو عبر منفذ SOCKS لبرنامج [v2rayN](https://github.com/2dust/v2rayN) أثناء تشغيله.

<p align="center">
  <img src="screenshots/connect.png" width="260" alt="Connect">
  <img src="screenshots/servers.png" width="260" alt="Servers">
  <img src="screenshots/settings.png" width="260" alt="Settings">
</p>

## الميزات

**النفق والحماية**
- نفق TUN شامل (Wintun) لكل تطبيقات Windows، وليس للمتصفح فقط.
- مفتاح إيقاف (Windows Filtering Platform): إذا انقطع النفق أو الوكيل يبقى الإنترنت محجوبًا بدل الرجوع إلى الاتصال الحقيقي.
- حماية من تسرب IPv6 وDNS، حتى عند إيقاف مفتاح الإيقاف.
- DNS عن بُعد (FakeIP): لا تغادر طلبات DNS الحاسوب، ويحلّ الخادم الأسماء في بلد الخروج، فتظهر اختبارات تسرب DNS نظيفة.
- اختبار تسرب IP وWebRTC داخل التطبيق مع عرض عنوان IP الخارجي.
- اختياري: مطابقة المنطقة الزمنية لـ Windows مع بلد الخادم.
- أمان عند الإغلاق المفاجئ: عملية مراقبة تعيد المسارات وDNS وملف hosts والمنطقة الزمنية وإعداد الوكيل في Windows.

**الخوادم**
- نواة [sing-box](https://github.com/SagerNet/sing-box) مدمجة: VLESS (Reality، وXHTTP عبر Xray)، VMess، Trojan، Shadowsocks، Hysteria / Hysteria2، TUIC، WireGuard، NaiveProxy، SOCKS، HTTP، OpenVPN (.ovpn) وAnyConnect / OpenConnect / Fortinet / GlobalProtect.
- اشتراكات (روابط base64 وصيغ sing-box وClash) مع تحديث تلقائي.
- Ping وزمن الاستجابة الحقيقي واختبار السرعة لخادم واحد أو للجميع.
- «تلقائي (الأسرع)» مع تبديل تلقائي للخادم دون قطع الاتصالات المفتوحة.
- أو استخدم v2rayN: يعثر v2tun عليه وعلى نواته تلقائيًا.

**المزيد**
- وكيل محلي: SOCKS5 وHTTP على منفذ واحد قابل للضبط، مع إمكانية الوصول من الشبكة المحلية، ووضع «وكيل فقط» يضبط وكيل Windows بدل إنشاء نفق.
- تقسيم النفق حسب التطبيق (exe) وحسب النطاق / نطاق IP، بوضعَي «الكل ما عدا هذه» و«هذه فقط».
- أيقونة بجانب الساعة، تشغيل مع Windows (بدون نافذة UAC)، اتصال تلقائي.
- واجهة بالإنجليزية والفارسية والعربية والصينية.

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

## الترخيص

يُوزَّع v2tun بموجب [GNU General Public License v3.0](../LICENSE) لأنه يتضمن sing-box ومكتبات أخرى مرخصة بـ GPL. المكونات الخارجية وتراخيصها مذكورة في [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md).

يُقدَّم هذا البرنامج «كما هو» دون أي ضمان. أنت مسؤول عن استخدامه وفق قوانين بلدك وشروط مزوّد الوكيل.

</div>
