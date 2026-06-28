# RACOON-Pi v2

ssl-RACOON-Controller などから送信された指令値をもとに、高速に情報を受信・制御を行うロボット側ソフトウェアです。

**Pi 4B（UART）** と **Rock5A（SPI）** の2ボードに対応しています。ビルド時に `-tags` でボードを明示指定してください。

> **Note:** 旧 `rock5a` ブランチの実装は本リポジトリに統合済みです。`rock5a` ブランチは廃止しました。

## ディレクトリ構成

```
cmd/
  racoon-pi2/          # メインエントリポイント
  dip_test/            # Rock5A DIP 診断ツール
  spi_test/            # Rock5A SPI 診断ツール
internal/
  app/                 # 起動・goroutine オーケストレーション
  state/               # 共有状態・データ構造
  link/                # UART/SPI 共通リンクロジック
  receive/             # AI / カメラ UDP 受信
  mw/                  # RACOON-MW へのマルチキャスト送信
  api/                 # HTTP API
  upgrade/             # 自動アップデート
  pi4/                 # Pi 4B 専用（UART, go-rpio）
  rock5a/              # Rock5A 専用（SPI, rock5a-gpio-go, sysfs PWM）
proto/                 # Protobuf 定義・生成コード
camera/                # カメラ処理（Python）
  capture/             # ボード別カメラ入力（pi4=Picamera2 / rock5a=V4L2）
  detect/              # color.py（HSV 検出）, calib.py（YOLO キャリブ）
  transport/           # UDP 送信・エンコード
  yolo/                # git submodule: Rione/ssl-YOLO-Detection
```

## クローン

カメラの YOLO モデルは git submodule（[Rione/ssl-YOLO-Detection](https://github.com/Rione/ssl-YOLO-Detection)）として `camera/yolo/` に含まれます。submodule ごと取得してください。

```bash
git clone --recurse-submodules https://github.com/Rione/ssl-RACOON-Pi2.git

# 既にクローン済みの場合
git submodule update --init --recursive
```

## ビルド

```bash
# Raspberry Pi 4B（UART / go-rpio）
go build -tags pi4 -o racoon-pi2 ./cmd/racoon-pi2

# Rock5A（SPI / rock5a-gpio-go）
go build -tags rock5a -o racoon-pi2 ./cmd/racoon-pi2

# Rock5A 診断ツール
go build -tags rock5a -o dip-test ./cmd/dip_test
go build -tags rock5a -o spi_test ./cmd/spi_test
```

タグ未指定の `go build .` は不可です。

## カメラ

カメラ処理は Python の `camera/` パッケージが担当します。通常運転では軽量な HSV + 輪郭検出のみを行い、検出結果を UDP（ポート 31133）で Go 本体へ送信します。Go 本体は起動時に `python3 -m camera` を実行し、ビルドタグに応じて環境変数 `RACOON_BOARD`（`pi4` / `rock5a`）を渡します。

### ボード別のカメラ入力

| ボード | バックエンド | デバイス |
| ------ | ------------ | -------- |
| Pi 4B | Picamera2（MIPI CSI） | Picamera2 既定 |
| Rock5A | OpenCV V4L2 | `/dev/video11`（既定）。`threshold.json` の `cameraDevice` で上書き可 |

### 依存パッケージ

```bash
pip install -r camera/requirements.txt
```

`picamera2` は Pi 4B のみ必要です。`ultralytics`（YOLO）はキャリブレーション時のみ遅延 import されます。

### ボール色キャリブレーション（`/calibballcolor`）

Raspberry Pi 上で常時 YOLO を動かすのは負荷が高いため、YOLO はキャリブレーション時のみ使用します。`GET /calibballcolor` を叩くと、カメラプロセスが 1 フレームを YOLO で推論し、検出したボールのバウンディングボックス中心と上下左右 4 点（計 5 点）から HSV を算出して `threshold.json` を更新します。以降は通常の HSV 検出が新しいしきい値で動作します（プロセス再起動不要）。

```bash
# ボールをカメラに写した状態で実行
curl http://<robot>:9191/calibballcolor
```

成功時はしきい値・バウンディングボックス・サンプル点・プレビュー画像（base64 JPEG）を含む JSON を返します。ボール未検出時は HTTP 400 を返します。

## 自動アップデート

GitHub Release からボード別バイナリを取得します。Public リポジトリのため `.env` や `GITHUB_TOKEN` は必須ではありません。`.env` がある場合は自動で読み込みます（API レート制限を避けたい場合に `GITHUB_TOKEN` を設定できます）。

| ビルド | Release アセット名（例） | フィルタ |
|--------|-------------------------|----------|
| Pi 4B | `racoon-pi2-pi4_v1.0.0_linux_arm64.tar.gz` | `^racoon-pi2-pi4_` |
| Rock5A | `racoon-pi2-rock5a_v1.0.0_linux_arm64.tar.gz` | `^racoon-pi2-rock5a_` |

同一バージョンタグ（例: `v1.0.0`）の Release に両方のアセットが含まれますが、実行中のビルドに応じて正しい方のみが選ばれます。

## Robot IDの決定方法

ロボットIDには、ロボットに内蔵されたDIPスイッチよりIDの検出を行います。
カバーと同じ色にIDを設定するようにしてください。

---

## Pi 4B（UART）

Raspberry Pi 4B 向け。STM との通信は UART（`/dev/serial0` @ 230400 baud）です。

### PIN ASSIGN / ピン配置

| 名称 | ピン番号/ポート名 |
| ------------- | ------------- |
| Serial(UART)  | /dev/serial0  |
| LED 1         | GPIO 18       |
| LED 2         | GPIO 27       |
| Button 1      | GPIO 22       |
| Button 2      | GPIO 24       |
| Buzzer        | GPIO 13(PWM)  |
| DIP 1         | GPIO 4        |
| DIP 2         | GPIO 5        |
| DIP 3         | GPIO 6        |
| DIP 4         | GPIO 25       |

UART を使用する際には設定が必要です。

```
sudo raspi-config
```

---

## Rock5A（SPI）

Radxa Rock5A 向け。STM との通信は SPI Master（`/dev/spidev4.0` @ 1 MHz, Mode0）です。

### PIN ASSIGN / ピン配置

| 名称 | Rock5A GPIO | 物理ピン |
| ------------- | ------------- | ------------- |
| SPI           | /dev/spidev4.0 | - |
| LED 1         | GPIO4_A1 (bank4,portA,pin1) | Pin 12 |
| LED 2         | GPIO4_B2 (bank4,portB,pin2) | Pin 13 |
| Button 1      | GPIO4_B4 | Pin 15 |
| Button 2      | GPIO1_B0 | Pin 18 |
| Buzzer (PWM)  | Pin11 = PWM15 | Pin 11 |
| DIP 1         | GPIO1_B3 | Pin 7 |
| DIP 2         | GPIO1_B2 | Pin 29 |
| DIP 3         | GPIO1_B1 | Pin 31 |
| DIP 4         | GPIO1_B5 | Pin 22 |

ブザー PWM にはデバイスツリーオーバーレイ `rk3588-pwm15-m1` の有効化が必要です。

初期ホスト名 `DietPi` の場合、初回起動時に `racoon-XXXXX` 形式のホスト名へ自動変更されます。
