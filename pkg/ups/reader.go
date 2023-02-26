package ups

import (
	"github.com/cockroachdb/errors"
	"github.com/d2r2/go-i2c"
	"github.com/d2r2/go-logger"
)

const (
	// Config Register (R/W)
	regConfig byte = 0x00

	// SHUNT VOLTAGE REGISTER (R)
	regShuntVoltage byte = 0x01

	// BUS VOLTAGE REGISTER (R)
	regBusVoltage byte = 0x02

	// POWER REGISTER (R)
	regPower byte = 0x03

	// CURRENT REGISTER (R)
	regCurrent byte = 0x04

	// CALIBRATION REGISTER (R/W)
	regCalibration byte = 0x05
)

type BusVoltageRange uint16

const (
	// Constants for ``bus_voltage_range``
	RANGE_16V BusVoltageRange = 0x00 // set bus voltage range to 16V
	RANGE_32V BusVoltageRange = 0x01 // set bus voltage range to 32V (default)
)

type Gain uint16

const (
	// Constants for ``gain``
	DIV_1_40MV  Gain = 0x00 // shunt prog. gain set to  1, 40 mV range
	DIV_2_80MV  Gain = 0x01 // shunt prog. gain set to /2, 80 mV range
	DIV_4_160MV Gain = 0x02 // shunt prog. gain set to /4, 160 mV range
	DIV_8_320MV Gain = 0x03 // shunt prog. gain set to /8, 320 mV range
)

type ADCResolution uint16

const (
	// Constants for ``bus_adc_resolution`` or ``shunt_adc_resolution``
	ADCRES_9BIT_1S    ADCResolution = 0x00 // 9bit, 1 sample, 84us
	ADCRES_10BIT_1S   ADCResolution = 0x01 // 10bit, 1 sample, 148us
	ADCRES_11BIT_1S   ADCResolution = 0x02 // 11 bit, 1 sample, 276us
	ADCRES_12BIT_1S   ADCResolution = 0x03 // 12 bit, 1 sample, 532us
	ADCRES_12BIT_2S   ADCResolution = 0x09 // 12 bit, 2 samples, 1.06ms
	ADCRES_12BIT_4S   ADCResolution = 0x0A // 12 bit, 4 samples, 2.13ms
	ADCRES_12BIT_8S   ADCResolution = 0x0B // 12bit, 8 samples, 4.26ms
	ADCRES_12BIT_16S  ADCResolution = 0x0C // 12bit, 16 samples, 8.51ms
	ADCRES_12BIT_32S  ADCResolution = 0x0D // 12bit, 32 samples, 17.02ms
	ADCRES_12BIT_64S  ADCResolution = 0x0E // 12bit, 64 samples, 34.05ms
	ADCRES_12BIT_128S ADCResolution = 0x0F // 12bit, 128 samples, 68.10ms
)

type Mode uint16

const (
	// Constants for ``mode``
	POWERDOW             Mode = 0x00 // power down
	SVOLT_TRIGGERED      Mode = 0x01 // shunt voltage triggered
	BVOLT_TRIGGERED      Mode = 0x02 // bus voltage triggered
	SANDBVOLT_TRIGGERED  Mode = 0x03 // shunt and bus voltage triggered
	ADCOFF               Mode = 0x04 // ADC off
	SVOLT_CONTINUOUS     Mode = 0x05 // shunt voltage continuous
	BVOLT_CONTINUOUS     Mode = 0x06 // bus voltage continuous
	SANDBVOLT_CONTINUOUS Mode = 0x07 // shunt and bus voltage continuous
)

type UPSManager struct {
	bus        *i2c.I2C
	addr       uint8
	calValue   uint16
	currentLSB float64
	powerLSB   float64
}

func NewManager() *UPSManager {
	logger.ChangePackageLogLevel("i2c", logger.WarnLevel)
	i2c, err := i2c.NewI2C(0x43, 1)
	if err != nil {
		panic("unable to initialize I2C reader")
	}
	ups := &UPSManager{
		bus: i2c,
	}

	ups.calValue = 0
	ups.currentLSB = 0
	ups.powerLSB = 0
	ups.SetCalibration16V5A()

	return ups
}

func (um *UPSManager) Close() error {
	return um.bus.Close()
}

func (um *UPSManager) Read(address uint8) (uint16, error) {
	data, _, err := um.bus.ReadRegBytes(address, 2)
	if err != nil {
		return 0, err
	}
	return uint16(data[0])*256 + uint16(data[1]), nil
}

func (um *UPSManager) Write(address uint8, data uint16) error {
	if err := um.bus.WriteRegU16BE(address, data); err != nil {
		return err
	}
	return nil
}

func (i *UPSManager) SetCalibration16V5A() {
	// By default we use a pretty huge range for the input voltage,
	// which probably isn't the most appropriate choice for system
	// that don't use a lot of power. But all of the calculations
	// are shown below if you want to change the settings. You will
	// also need to change any relevant register settings, such as
	// setting the VBUS_MAX to 16V instead of 32V, etc.
	// VBUS_MAX = 16V             (Assumes 16V, can also be set to 32V)
	// VSHUNT_MAX = 0.08          (Assumes Gain 2, 80mV, can also be 0.32, 0.16, 0.04)
	// RSHUNT = 0.01               (Resistor value in ohms)

	// 1. Determine max possible current
	// MaxPossible_I = VSHUNT_MAX / RSHUNT
	// MaxPossible_I = 8.0A

	// 2. Determine max expected current
	// MaxExpected_I = 5.0A

	// 3. Calculate possible range of LSBs (Min = 15-bit, Max = 12-bit)
	// MinimumLSB = MaxExpected_I/32767
	// MinimumLSB = 0.0001529              (61uA per bit)
	// MaximumLSB = MaxExpected_I/4096
	// MaximumLSB = 0,0012207              (488uA per bit)

	// 4. Choose an LSB between the min and max values
	//    (Preferrably a roundish number close to MinLSB)
	// CurrentLSB = 0.00016 (uA per bit)
	i.currentLSB = 0.1524 // Current LSB = 100uA per bit

	// 5. Compute the calibration register
	// Cal = trunc (0.04096 / (Current_LSB * RSHUNT))
	// Cal = 13434 (0x347a)
	i.calValue = 26868

	// 6. Calculate the power LSB
	// PowerLSB = 20 * CurrentLSB
	// PowerLSB = 0.002 (2mW per bit)
	i.powerLSB = 0.003048 // Power LSB = 2mW per bit

	// 7. Compute the maximum current and shunt voltage values before overflow
	//
	// Max_Current = Current_LSB * 32767
	// Max_Current = 3.2767A before overflow
	//
	// If Max_Current > Max_Possible_I then
	//    Max_Current_Before_Overflow = MaxPossible_I
	// Else
	//    Max_Current_Before_Overflow = Max_Current
	// End If
	//
	// Max_ShuntVoltage = Max_Current_Before_Overflow * RSHUNT
	// Max_ShuntVoltage = 0.32V
	//
	// If Max_ShuntVoltage >= VSHUNT_MAX
	//    Max_ShuntVoltage_Before_Overflow = VSHUNT_MAX
	// Else
	//    Max_ShuntVoltage_Before_Overflow = Max_ShuntVoltage
	// End If

	// 8. Compute the Maximum Power
	// MaximumPower = Max_Current_Before_Overflow * VBUS_MAX
	// MaximumPower = 3.2 * 32V
	// MaximumPower = 102.4
	i.Write(regCalibration, i.calValue)

	busVoltageRange := uint16(RANGE_16V)
	gain := uint16(DIV_2_80MV)
	busAdcResolution := uint16(ADCRES_12BIT_32S)
	shuntADCResolution := uint16(ADCRES_12BIT_32S)
	mode := uint16(SANDBVOLT_CONTINUOUS)

	// Set Config register to take into account the settings above
	config := busVoltageRange<<13 |
		gain<<11 |
		busAdcResolution<<7 |
		shuntADCResolution<<3 |
		mode
	i.Write(regConfig, config)
}

func (um *UPSManager) GetShuntVoltage_mV() (float32, error) {
	err := um.Write(regCalibration, um.calValue)
	if err != nil {
		return 0, err
	}
	data, err := um.Read(regShuntVoltage)
	if err != nil {
		return 0, err
	}

	return float32(data) * 0.01, nil
}

func (um *UPSManager) GetBusVoltage_V() (float32, error) {
	err := um.Write(regCalibration, um.calValue)
	if err != nil {
		return 0, errors.Wrapf(err, "Error writing to buffer")
	}
	data, err := um.Read(regBusVoltage)
	if err != nil {
		return 0, err
	}
	return float32((data >> 3)) * 0.004, nil
}

func (um *UPSManager) GetCurrent_mA() (float32, error) {
	data, err := um.Read(regCurrent)
	if err != nil {
		return 0, err
	}
	// This is a trick. This will be close to the max uint16 when
	// the raspberry pi is using the battery. This means that when
	// we substract maxInt, the value will go negative.
	dataInt := int(data)
	if dataInt > 32767 {
		dataInt -= 65535
	}
	return float32(dataInt) * float32(um.currentLSB), nil
}

func (um *UPSManager) GetPower_W() (float32, error) {
	err := um.Write(regCalibration, um.calValue)
	if err != nil {
		return 0, err
	}
	data, err := um.Read(regPower)
	if err != nil {
		return 0, err
	}
	if data > 32767 {
		data -= 65535
	}
	return float32(data) * float32(um.powerLSB), nil
}
