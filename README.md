# RACOON-Pi v2
ssl-RACOON-Controllerなどから送信された指令値をもとに、
高速に情報を受信・制御を行うための Rock5A 側ソフトウェアです。

## Robot IDの決定方法
ロボットIDには、ロボットに内蔵されたDIPスイッチよりIDの検出を行います。
カバーと同じ色にIDを設定するようにしてください。


## PIN ASSIGN / ピン配置
FOR RADXA ROCK 5A

ロボットのドリブラ、およびキッカーのピン配置は以下の通りです。

| 名称 | 物理ピン | Rock5A GPIO番号 | Rock5A GPIO名 | (旧RPi BCM) |
| ------------- | ------- | -------------- | ------------- | ----------- |
| Serial(UART)  | Pin 8/10 | GPIO 13/14    | GPIO0_B5/B6 (UART2) | GPIO 14/15 |
| LED 1         | Pin 12  | GPIO 129       | GPIO4_A1      | GPIO 18     |
| LED 2         | Pin 13  | GPIO 138       | GPIO4_B2      | GPIO 27     |
| Button 1      | Pin 15  | GPIO 140       | GPIO4_B4      | GPIO 22     |
| Button 2      | Pin 18  | GPIO 40        | GPIO1_B0      | GPIO 24     |
| Buzzer (PWM)  | **Pin 11** | GPIO 139    | GPIO4_B3 (PWM15) | GPIO 13 (Pin33) |
| DIP 1         | Pin 7   | GPIO 43        | GPIO1_B3      | GPIO 4      |
| DIP 2         | Pin 29  | GPIO 42        | GPIO1_B2      | GPIO 5      |
| DIP 3         | Pin 31  | GPIO 41        | GPIO1_B1      | GPIO 6      |
| DIP 4         | Pin 22  | GPIO 45        | GPIO1_B5      | GPIO 25     |

> **注意**: ブザーは RPi の Pin 33 から Rock5A の **Pin 11** に変更されています。
> Rock5A の Pin 33 にはハードウェア PWM 機能がないため、PWM15 が使える Pin 11 を使用します。

UARTにおけるボーレートは 230400　です。

キッカー・ドリブラ・各ホイールへの通信はUARTによって行われます。

## セットアップ（Rock5A / DietPi）

### 1. デバイスツリーオーバーレイの設定

`/boot/dietpiEnv.txt` を編集し、以下のオーバーレイを有効にしてください。

```
overlays=rk3588-pwm15-m1 rk3588-uart2-m0
```

- `rk3588-pwm15-m1`: ブザー用 PWM15（Pin 11）を有効化
- `rk3588-uart2-m0`: UART2（Pin 8/10）をシリアル通信用に有効化

設定後、再起動してください。

### 2. fiq-debuggerの無効化

デフォルトでは UART2 がデバッグコンソール（fiq-debugger）に使用されています。
シリアル通信で使用するために、fiq-debugger を別の UART に移動するか無効化する必要があります。

### 3. GPIOライブラリ

GPIO制御には [Yuzz1e/rock5a-gpio-go](https://github.com/Yuzz1e/rock5a-gpio-go)（RK3588/ROCK 5A 用 sysfs + MMIO プル制御）を使用しています。`OpenGPIO(bank, port, pin)` でピンを export し、`SetDirection` / `Read` / `Write` で入出力を制御します。
PWM制御は `/sys/class/pwm` を直接操作する方式です。

### 4. ビルドと実行

```bash
go build -o racoon-pi2 .
sudo ./racoon-pi2
```

GPIO および PWM の操作には root 権限が必要です。
