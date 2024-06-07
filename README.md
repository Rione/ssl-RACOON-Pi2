# RACOON-Pi v2
ssl-RACOON-Controllerなどから送信された指令値をもとに、
高速に情報を受信・制御を行うためのRaspberry Pi 4B側ソフトウェアです。

## Robot IDの決定方法
ロボットIDには、ロボットに内蔵されたDIPスイッチよりIDの検出を行います。
カバーと同じ色にIDを設定するようにしてください。


## PIN ASSIGN / ピン配置
FOR RASPBERRY PI ONLY  
ロボットのドリブラ、およびキッカーのピン配置は以下の通りです。  
|      名称      | ピン番号/ポート名 |
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

UARTにおけるボーレートは 230400　です。

キッカー・ドリブラ・各ホイールへの通信はUARTによって行われます。  
UARTを使用する際には、設定が必要です。下記より、設定を行ってください。
```
sudo raspi-config
```
