# opencode2api: panduan pemula

[English](README.md) | [简体中文](README.zh-CN.md) | [Tiếng Việt](README.vi.md) | **Bahasa Indonesia**

Dokumen ini ditulis untuk orang yang **belum pernah** memakai proyek ini. Semua perintah di sini bisa disalin apa adanya. Setiap nama setelan, port, dan pesan error diambil dari file di repo ini, bukan dari perkiraan.

Kalau Anda hanya ingin tahu "apa ini", baca tiga baris berikut:

1. Ini satu program (gateway) yang berdiri di antara aplikasi Anda dan layanan **OpenCode Zen / Zen Go**.
2. Aplikasi Anda bicara ke program ini memakai format OpenAI, Responses, atau Anthropic. Program ini yang meneruskan ke OpenCode memakai key Anda.
3. Program ini juga punya **WebUI** untuk mengatur key, melihat statistik, dan mengetes model lewat browser.

---

## 1. Apa yang Anda butuhkan

| Kebutuhan | Kenapa | Cara memeriksa |
| --- | --- | --- |
| Windows, Linux, atau macOS | Program ini bisa dijalankan di ketiganya | - |
| Go 1.24 atau lebih baru | Hanya kalau Anda build sendiri dari kode | `go version` |
| Key OpenCode Zen atau Go | Supaya permintaan diteruskan ke upstream | Dari akun OpenCode Anda |
| Browser | Untuk membuka WebUI | - |

Catatan penting:

- **Tidak perlu database.** Tidak ada langkah migrasi atau seed.
- **Tidak perlu Node.js.** WebUI sudah tertanam di dalam file binary.
- **Key upstream bersifat opsional.** Kalau Anda tidak punya key OpenCode, set `"anonymous": true` di config dan gateway memakai jalur gratis tanpa key. Konsekuensinya: lebih sering kena batas laju (rate limit).
- Kalau Anda memakai Docker, Go tidak perlu dipasang. Lihat bagian 11.

## 2. Ambil kode dan siapkan config

```bash
git clone https://github.com/mulqizamzam/opencode2api.git
cd opencode2api
cp config.example.json config.json
```

Di Windows (PowerShell):

```powershell
git clone https://github.com/mulqizamzam/opencode2api.git
cd opencode2api
Copy-Item config.example.json config.json
```

`config.json` sengaja tidak ikut masuk git (lihat `.gitignore`), karena berisi rahasia Anda. File contohnya (`config.example.json`) berisi nilai palsu yang harus diganti.

## 3. Build

Kalau Anda memakai binary siap pakai dari halaman Releases repo ini, lewati bagian ini.

Linux atau macOS:

```bash
go build -o opencode2api ./cmd/opencode2api
```

Windows:

```powershell
go build -o opencode2api.exe ./cmd/opencode2api
```

Kalau perintah ini selesai tanpa pesan error, file `opencode2api` (atau `opencode2api.exe`) sudah ada di folder yang sama dengan `config.json`.

## 4. Isi config

Buka `config.json` dengan editor teks. **Lima hal** ini yang wajib Anda perhatikan lebih dulu:

| Setelan | Artinya | Contoh aman | Aturan |
| --- | --- | --- | --- |
| `server_keys` | Key yang dipakai aplikasi Anda untuk masuk ke gateway ini. Bukan key OpenCode. | `["ganti-key-lokal-saya"]` | Minimal 1 isi, tidak boleh kosong |
| `zen_keys` | Key upstream OpenCode Zen | `["sk-key-zen-anda"]` | Boleh kosong kalau `anonymous: true` |
| `go_keys` | Key upstream OpenCode Go | `[]` | Boleh kosong kalau `anonymous: true` |
| `anonymous` | Pakai jalur gratis tanpa key | `true` atau `false` | Kalau `false`, minimal satu dari `zen_keys` / `go_keys` harus berisi |
| `webui.password` | Password login WebUI | `"ganti-password-ini"` | Minimal 10 karakter |

Contoh potongan yang sudah benar untuk pemakaian pertama:

```json
{
  "listen": "127.0.0.1:8080",
  "server_keys": ["ganti-key-lokal-saya"],
  "zen_keys": [],
  "go_keys": [],
  "anonymous": true,
  "webui": {
    "enabled": true,
    "listen": "0.0.0.0:8081",
    "username": "admin",
    "password": "ganti-password-ini",
    "session_ttl_minutes": 720
  }
}
```

Saat pertama kali dijalankan, gateway mengubah `webui.password` menjadi `webui.password_hash` dan mengosongkan password polosnya. Jadi setelah sekali jalan, yang tersimpan di file adalah hash, bukan password asli.

Setelan lain (boleh dibiarkan seperti contoh):

| Setelan | Artinya | Nilai contoh |
| --- | --- | --- |
| `listen` | Alamat API yang dipakai aplikasi Anda | `127.0.0.1:8080` |
| `webui.listen` | Alamat WebUI | `0.0.0.0:8081` |
| `webui.enabled` | Nyalakan WebUI | `true` |
| `webui.username` | Nama pengguna WebUI | `admin` |
| `webui.session_ttl_minutes` | Umur sesi login | `720` (12 jam) |
| `proxies` | Jalur keluar. `direct` berarti langsung tanpa proxy | `["direct"]` |
| `proxyfile` | File daftar proxy, satu per baris | `""` |
| `upstream.zen` / `upstream.go` / `upstream.anonymous` | Alamat layanan OpenCode | lihat contoh |
| `retry.max_attempts` | Berapa kali dicoba ulang | `3` |
| `retry.timeout_seconds` | Batas waktu satu permintaan | `300` |
| `models.refresh_seconds` | Seberapa sering daftar model disegarkan | `300` |
| `models.protocols` | Paksa protokol untuk model tertentu | `{}` |
| `models.free_only` | Hanya tampilkan model gratis | `false` |
| `performance.*` | Setelan koneksi dan timeout | lihat contoh |
| `logging.level` | `debug`, `info`, `warn`, atau `error` | `info` |
| `logging.ring_size` | Jumlah baris log yang disimpan untuk WebUI | `2000` |
| `prefer` | Tier mana yang dicoba lebih dulu: `go` atau `zen` | `go` |
| `reasoning.effort` | Tingkat berpikir bawaan: `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, `auto`, atau `none` | `high` |
| `rotation.*` | Grup model `sota` dan `sweet` untuk failover otomatis | lihat contoh |

Catatan untuk setelan angka bersatuan detik: nilai maksimumnya 86400 (satu hari). Nilai lebih besar ditolak saat start, supaya tidak ada perhitungan waktu yang rusak.

## 5. Database dan layanan lain

Tidak ada. Proyek ini tidak memakai database, tidak ada langkah inisialisasi, dan tidak butuh layanan tambahan. Yang perlu hanya koneksi internet ke layanan OpenCode.

## 6. Jalankan

Linux atau macOS:

```bash
./opencode2api -config config.json
```

Windows:

```powershell
.\opencode2api.exe -config config.json
```

Kalau berhasil, di console akan muncul dua baris seperti ini (alamatnya sesuai config Anda):

```json
{"level":"INFO","msg":"server listening","component":"api","event":"server_started","address":"127.0.0.1:8080","version":"dev"}
{"level":"INFO","msg":"server listening","component":"webui","event":"server_started","address":"0.0.0.0:8081","version":"dev"}
```

Artinya:

- API siap di `http://127.0.0.1:8080` (dipakai aplikasi Anda).
- WebUI siap di `http://localhost:8081` (dibuka di browser).

Untuk menghentikan program, tekan `Ctrl+C` di jendela console itu.

Opsi tambahan saat menjalankan:

| Opsi | Artinya |
| --- | --- |
| `-config config.json` | Lokasi file config. Kalau tidak ditulis, bawaan `config.json` |
| `-listen 127.0.0.1:6868` | Menimpa alamat API tanpa mengubah file config |
| `-web-listen 0.0.0.0:6869` | Menimpa alamat WebUI tanpa mengubah file config |

## 7. Pemakaian pertama

### 7a. Masuk ke WebUI

1. Buka `http://localhost:8081`.
2. Isi nama pengguna (`admin` kalau Anda tidak mengubahnya) dan password yang Anda tulis di `webui.password`.
3. Setelah masuk, Anda akan melihat menu: 概览 (ringkasan), 诊断 (diagnostik), 接入指南 (panduan koneksi), API Key 管理, 模型与额度 (model dan kuota), 模型轮转 (rotasi model), 配置中心 (config), 账号安全 (akun).
4. Bahasa antarmuka bisa diganti dari pemilih bahasa di kanan atas: 简体中文 (bawaan), English, Tiếng Việt.

Halaman 配置中心 bisa dipakai untuk mengubah config lewat browser: tekan simpan, dan perubahan yang bisa diterapkan tanpa restart akan langsung berlaku.

### 7b. Tes API lewat terminal

Cek kesehatan gateway. Endpoint ini tidak butuh key:

```bash
curl http://127.0.0.1:8080/healthz
```

Daftar model yang bisa dipakai. Ganti `ganti-key-lokal-saya` dengan isi `server_keys` Anda:

```bash
curl http://127.0.0.1:8080/v1/models \
  -H "Authorization: Bearer ganti-key-lokal-saya"
```

Ambil satu `id` dari daftar itu, lalu kirim pertanyaan. Ganti `MODEL_ID` dengan id tersebut:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer ganti-key-lokal-saya" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL_ID","messages":[{"role":"user","content":"Halo"}]}'
```

Balasannya berbentuk JSON dengan isi jawaban di `choices[0].message.content`, dan pemakaian token di `usage`. Setiap balasan juga membawa header `x-request-id`, berguna saat mencari baris log permintaan itu.

Untuk jawaban mengalir (streaming), tambahkan `"stream": true` pada body dan pakai `curl -N`.

### 7c. Pakai dari aplikasi Anda

Aplikasi apa pun yang mendukung API OpenAI bisa diarahkan ke gateway ini:

| Setelan di aplikasi | Nilai |
| --- | --- |
| Base URL | `http://127.0.0.1:8080/v1` |
| API key | salah satu isi `server_keys` Anda |
| Model | salah satu id dari `/v1/models` |

Klien bergaya Anthropic memakai header `x-api-key` dan path `/v1/messages`.

Endpoint yang tersedia:

| Metode | Path | Keterangan |
| --- | --- | --- |
| GET | `/v1/models` | Model yang bisa dipakai dengan config saat ini |
| POST | `/v1/chat/completions` | Format Chat Completions |
| POST | `/v1/responses` | Format Responses |
| POST | `/v1/messages` | Format Anthropic Messages |
| POST | `/v1/systemone` | Model System One (payload diteruskan apa adanya) |
| GET | `/healthz` | Status kesiapan, tanpa autentikasi |

## 8. Struktur proyek

```
opencode2api/
├── cmd/opencode2api/main.go      Titik masuk: baca config, jalankan API dan WebUI
├── internal/
│   ├── admin/                    Handler WebUI dan API pengelolaan
│   ├── config/                   Baca, validasi, simpan config
│   ├── gateway/                  Inti: pilih key, proxy, percobaan ulang, rotasi
│   ├── models/                   Daftar model, harga, cache di disk
│   ├── protocol/                 Terjemahan antar format dan parsing aliran SSE
│   ├── rotation/                 Status grup model sota dan sweet
│   └── telemetry/                Log, metrik, dan pemulihan dari panic
├── webui/                        Berkas WebUI (HTML, CSS, JS) yang ditanam ke binary
├── docs/                         Dokumen tambahan
├── config.example.json           Contoh config. Salin ke config.json
├── compose.yaml                  Jalur Docker
└── Dockerfile
```

Berkas yang paling sering Anda sentuh sebagai pemula:

- `config.json` (Anda buat sendiri dari contoh).
- `internal/config/config.go` kalau Anda ingin menambah setelan baru.
- `webui/` kalau Anda ingin mengubah tampilan WebUI.

## 9. Perintah yang sering dipakai

| Keperluan | Perintah |
| --- | --- |
| Build | `go build -o opencode2api ./cmd/opencode2api` |
| Build seperti rilis | `go build -trimpath ./cmd/opencode2api` |
| Jalankan | `./opencode2api -config config.json` |
| Semua test | `go test ./...` |
| Test satu paket | `go test ./internal/protocol/` |
| Periksa kode | `go vet ./...` |
| Rapikan format | `gofmt -w .` |
| Cek format tanpa mengubah | `gofmt -l .` |
| Cek sintaks entrypoint Docker | `sh -n docker-entrypoint.sh` |

## 10. Test

```bash
go test ./...
```

Hasil yang menandakan lulus adalah baris `ok` per paket, misalnya:

```
ok  	opencode2api/internal/config	0.7s
ok  	opencode2api/internal/gateway	16.0s
ok  	opencode2api/internal/protocol	0.9s
```

Paket tanpa berkas test akan muncul sebagai `no test files`, dan itu bukan kegagalan.

Catatan jujur: rangkaian pemeriksaan otomatis di `.github/workflows/checks.yml` saat ini menjalankan `go vet ./...`, `go build -trimpath ./cmd/opencode2api`, pemeriksaan format Go, pemeriksaan sintaks WebUI, dan `sh -n docker-entrypoint.sh`. Test belum dijalankan otomatis di sana, jadi jalankan `go test ./...` sendiri sebelum mengirim perubahan.

## 11. Menjalankan dengan Docker

Cara ini tidak butuh Go.

```bash
cp config.example.json config.json
# sunting config.json: isi server_keys dan webui.password
docker compose up -d
docker compose logs -f
```

Variabel yang bisa diatur lewat environment:

| Variabel | Bawaan | Artinya |
| --- | --- | --- |
| `OPENCODE2API_VERSION` | `latest` | Tag image |
| `OPENCODE2API_PORT` | `8080` | Port host untuk API |
| `OPENCODE2API_WEBUI_PORT` | `8081` | Port host untuk WebUI |
| `OPENCODE2API_LISTEN` | kosong | Menimpa alamat API di dalam container |
| `OPENCODE2API_WEBUI_LISTEN` | kosong | Menimpa alamat WebUI di dalam container |

Di dalam container, entrypoint membaca `CONFIG_PATH`, `CONFIG_SEED_PATH`, `LISTEN_ADDRESS`, dan `WEBUI_LISTEN_ADDRESS`. Volume bernama `opencode2api-state` menyimpan config hasil suntingan, dan isi host hanya disalin ke sana pada start pertama.

## 12. Kalau ada masalah

Pesan di bawah ini diambil apa adanya dari kode validasi config, jadi Anda bisa mencocokkannya langsung.

| Yang Anda lihat | Penyebab | Cara memeriksa | Perbaikan |
| --- | --- | --- | --- |
| `configuration error` diikuti `read config.json: ... no such file or directory` | `config.json` belum ada | `ls config.json` | Salin dari contoh: `cp config.example.json config.json` |
| `webui.password must contain at least 10 characters` | Password WebUI kurang dari 10 karakter | Lihat nilai `webui.password` | Pakai minimal 10 karakter |
| `server_keys must contain at least one local key` | Daftar key lokal kosong | Lihat `server_keys` | Isi minimal satu key |
| `zen_keys or go_keys must contain at least one upstream key unless anonymous is enabled` | Tidak ada key upstream dan mode anonymous mati | Lihat `anonymous` | Isi `zen_keys`/`go_keys`, atau set `"anonymous": true` |
| `至少保留一个启用的 API Key` | Semua key lokal dinonaktifkan dari WebUI | Halaman API Key 管理 | Aktifkan minimal satu key |
| `* must be at most 86400` | Nilai detik terlalu besar | Lihat setelan detik yang disebut | Pakai nilai maksimum 86400 |
| `listen tcp 127.0.0.1:8080: bind: address already in use` | Port sudah dipakai program lain | Ganti port, atau cari pemakai port itu | Ubah `listen`, atau jalankan dengan `-listen 127.0.0.1:6868` |
| Balasan API `invalid local API key` | Key di aplikasi Anda tidak sama dengan `server_keys` | Bandingkan keduanya | Samakan, atau buat key baru di halaman API Key 管理 |
| Balasan API `the model uses an upstream protocol that opencode2api does not expose` | Model itu protokolnya tidak didukung | `GET /v1/models` | Pakai model yang muncul di daftar |
| Balasan API `502` dengan `all upstream attempts failed` | Semua percobaan ke upstream gagal: key habis kuota, proxy mati, atau upstream sedang bermasalah | `GET /healthz`, lalu halaman 诊断 | Periksa key dan proxy. Untuk melihat sebabnya, lihat 诊断, tab 事件流 |
| Balasan API `504` dengan `upstream request timed out` | Waktu tunggu habis | Lihat `retry.timeout_seconds` | Naikkan nilainya, misalnya `600` |
| Lupa password WebUI | Password hanya tersimpan sebagai hash | Lihat `webui.password_hash` | Tulis password baru di `webui.password` (minimal 10 karakter), lalu restart. Gateway akan meng-hash ulang |

## 13. Panduan pengembangan

1. Buat branch baru dari `main`.
2. Ubah kode.
3. Jalankan `gofmt -l .` (harus tidak mengeluarkan apa pun), `go vet ./...`, lalu `go test ./...`.
4. Kalau Anda mengubah berkas di `webui/`, ingat bahwa berkas itu ditanam ke binary saat build, jadi build ulang untuk melihat hasilnya.
5. Commit dengan pesan yang jelas, lalu buka pull request.

## 14. Deployment

Pilihan paling sederhana untuk pemula adalah Docker Compose seperti pada bagian 11, karena config, port, dan volume sudah diatur di `compose.yaml`.

Kalau ingin menjalankan binary langsung di server:

1. Build dengan `go build -trimpath -ldflags="-s -w -X main.version=<tag>" -o opencode2api ./cmd/opencode2api`.
2. Salin `opencode2api` dan `config.json` ke server.
3. Jalankan sebagai layanan, misalnya lewat `systemd`, dengan `-config` menunjuk ke file config Anda.
4. Batasi akses ke port API dan WebUI. Jangan buka WebUI ke internet tanpa HTTPS.

## 15. Catatan keamanan

- `config.json` berisi `server_keys`, key upstream, dan hash password. Berkas ini sudah masuk `.gitignore`. Jangan pernah menempel isinya ke issue, chat, atau commit.
- Key upstream (`zen_keys`, `go_keys`) adalah rahasia. Kalau bocor, ganti di akun OpenCode Anda.
- WebUI tidak punya pembatasan selain password. Pakai hanya di jaringan tepercaya, atau balik proxy HTTPS, dan jangan meneruskan portnya ke internet.
- Kalau log dicetak, rahasia yang dikenal akan disamarkan, tetapi isi percakapan tetap bisa sensitif. Setelan `logging.dump_request_bodies` sebaiknya tetap `false` kecuali sedang menelusuri masalah.

## 16. Pertanyaan yang sering muncul

**Perlu database?**
Tidak. Tidak ada database dan tidak ada langkah migrasi.

**Perlu memasang Node.js?**
Tidak. WebUI ditanam di dalam binary.

**Bisa jalan tanpa key OpenCode?**
Bisa. Set `"anonymous": true`, dan biarkan `zen_keys` serta `go_keys` kosong.

**Bagaimana mengganti port?**
Ubah `listen` untuk API dan `webui.listen` untuk WebUI di `config.json`, lalu restart. Untuk sekali jalan tanpa mengubah file, pakai `-listen` dan `-web-listen`.

**Di mana saya bisa melihat log?**
Di console tempat program dijalankan, dan di WebUI pada 诊断 lalu tab 事件流. Jumlah baris yang disimpan diatur `logging.ring_size`.

**Apakah riwayat percakapan disimpan?**
Tidak. Gateway tidak menyimpan riwayat, jadi aplikasi Anda harus mengirim riwayatnya sendiri.

**Bagaimana cara memperbarui ke versi baru?**
Ambil kode terbaru, build ulang, lalu jalankan kembali dengan `config.json` yang sama. Config lama tetap kompatibel selama tidak memakai nama setelan yang sudah dihapus.

## 17. Daftar periksa terakhir

- [ ] `go version` menunjukkan 1.24 atau lebih baru (kalau build sendiri).
- [ ] `config.json` ada dan `server_keys` sudah diisi.
- [ ] `anonymous` bernilai `true`, atau `zen_keys`/`go_keys` sudah diisi.
- [ ] `webui.password` minimal 10 karakter.
- [ ] Program jalan tanpa `configuration error`.
- [ ] `http://127.0.0.1:8080/healthz` menjawab dengan status `ok`.
- [ ] WebUI terbuka di `http://localhost:8081` dan Anda bisa login.
- [ ] `GET /v1/models` mengembalikan daftar model.
- [ ] Satu permintaan ke `/v1/chat/completions` berhasil.
- [ ] `config.json` tidak ikut ter-commit ke git.

---

Bagian referensi yang lebih dalam ada di [README bahasa Inggris](README.md) dan [panduan konsol pengelolaan (`docs/management.zh-CN.md`)](docs/management.zh-CN.md).
