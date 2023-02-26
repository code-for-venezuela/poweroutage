package main

import (
	"fmt"
	"time"

	"github.com/code-for-venezuela/poweroutage/pkg/ups"
	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
)

func main() {
	logger.ChangePackageLogLevel("i2c", logger.WarnLevel)
	i2c, err := i2c.NewI2C(0x43, 1)
	if err != nil {
		fmt.Printf("Error: %v", err)
		return
	}
	defer i2c.Close()

	ina219 := ups.NewManager(i2c)

	for {
		busVoltage, err := ina219.GetBusVoltage_V()
		if err != nil {
			panic(err)
		}
		current, err := ina219.GetCurrent_mA()
		if err != nil {
			panic(err)
		}
		power, err := ina219.GetPower_W()
		if err != nil {
			panic(err)
		}

		p := (busVoltage - 3) / 1.2 * 100
		if p > 100 {
			p = 100
		}
		if p < 0 {
			p = 0
		}

		// INA219 measure bus voltage on the load side. So PSU voltage = bus_voltage + shunt_voltage
		fmt.Printf("Load Voltage:  %.3f V\n", busVoltage)
		fmt.Printf("Current:       %.3f A\n", current/1000)
		fmt.Printf("Power:         %.3f W\n", power)
		fmt.Printf("Percent:       %.1f%%\n", p)
		fmt.Println()

		time.Sleep(2 * time.Second)
	}
}
