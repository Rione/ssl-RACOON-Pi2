package link

//バッテリー電圧の判定。STMから来る値は１V刻みで上下に跳ねる為、
//一回の跳ねで警報が始まらないようにし、電圧が戻ったら解除できるようにする。

const (
	// 連続して閾値以下だった回数がこれに達したら警報を出す
	batteryDebounceCount = 5
	// 警報を解除する余裕。0.1V単位なので5=0.5V
	batteryRecoverMargin = 5
)

var batteryLowCount int

//BatteryBelowThresholdは、閾値以下が続いた時だけtrueを返す。
//voltが0のときはSTMからまだ値が届いていないので無視する。
func BatteryBelowThreshold(volt uint8, threshold int) bool {
	if volt == 0 {
		batteryLowCount = 0
		return false
	}
	if int(volt) <= threshold {
		if batteryLowCount < batteryDebounceCount {
			batteryLowCount++
		}
	} else {
		batteryLowCount = 0
	}
	return batteryLowCount >= batteryDebounceCount
}

//BatteryRecoveredは、警報を解除して良いほど電圧が戻ったかを返す。
func BatteryRecovered(volt uint8, threshold int) bool {
	if volt == 0 {
		return false
	}
	return int(volt) > threshold+batteryRecoverMargin
}

//ResetBatteryDebounceは警報を抜けるときに呼ぶ。
func ResetBatteryDebounce() {
	batteryLowCount = 0
}
