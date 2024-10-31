package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"reflect"
	"time"

	"github.com/google/gousb"
)

var (
	pollingFrequency = flag.Int("freq", 500, "Polling frequency in Hz")
	readonly         = flag.Bool("readonly", false, "Only read from the controller")
	debug            = flag.Int("debug", 0, "USB debugging control")
)

const (
	VendorMicrosoft  = 0x045e
	ProductXboxOne   = 0x02d1
	ProductXboxOneS  = 0x02dd
	ProductXboxOneX  = 0x02ea
	ProductXboxElite = 0x02e3
)

type Controller struct {
	device *gousb.Device
	config *gousb.Config
	intf   *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint
}

type ControllerState struct {
	A, B, X, Y, RB, LB, UP, RIGHT, DOWN, LEFT, LS, RS, MENU, VIEW, GUIDE, SHARE bool
	LT, RT, LEFTX, LEFTY, RIGHTX, RIGHTY                                        float32
	LastState                                                                   *ControllerState
}

func NewController() (*Controller, error) {
	ctx := gousb.NewContext()

	for _, pid := range []gousb.ID{ProductXboxOne, ProductXboxOneS, ProductXboxOneX, ProductXboxElite} {
		device, err := ctx.OpenDeviceWithVIDPID(VendorMicrosoft, pid)
		if err != nil {
			continue
		}

		if device == nil {
			continue
		}

		log.Printf("Found Xbox controller with PID: %#x", pid)

		config, err := device.Config(1)
		if err != nil {
			device.Close()
			continue
		}

		intf, err := config.Interface(0, 0)
		if err != nil {
			config.Close()
			device.Close()
			continue
		}

		in, err := intf.InEndpoint(1)
		if err != nil {
			intf.Close()
			config.Close()
			device.Close()
			continue
		}

		out, err := intf.OutEndpoint(1)
		if err != nil {
			intf.Close()
			config.Close()
			device.Close()
			continue
		}

		return &Controller{
			device: device,
			config: config,
			intf:   intf,
			in:     in,
			out:    out,
		}, nil
	}

	return nil, fmt.Errorf("no compatible Xbox controller found")
}

func (c *Controller) Close() {
	if c.intf != nil {
		c.intf.Close()
	}
	if c.config != nil {
		c.config.Close()
	}
	if c.device != nil {
		c.device.Close()
	}
}

func (c *Controller) Initialize() error {
	init := []byte{0x05, 0x20}
	_, err := c.out.Write(init)
	if err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	return nil
}

func (c *Controller) ReadState() (*ControllerState, error) {
	buf := make([]byte, 64)
	n, err := c.in.Read(buf)
	if err != nil {
		return nil, err
	}

	if n < 16 {
		return nil, fmt.Errorf("short read: %d bytes", n)
	}

	state := &ControllerState{}

	switch buf[0] {
	case 0x20:
		btn1 := buf[3]
		btn2 := buf[4]

		state.A = btn1&0x10 != 0
		state.B = btn1&0x40 != 0
		state.X = btn1&0x20 != 0
		state.Y = btn1&0x80 != 0
		state.MENU = btn1&0x04 != 0
		state.VIEW = btn1&0x08 != 0
		state.SHARE = btn1&0x01 != 0
		state.UP = btn2&0x01 != 0
		state.DOWN = btn2&0x02 != 0
		state.LEFT = btn2&0x04 != 0
		state.RIGHT = btn2&0x08 != 0
		state.LB = btn2&0x10 != 0
		state.RB = btn2&0x20 != 0
		state.LS = btn2&0x40 != 0
		state.RS = btn2&0x80 != 0
		lt := binary.LittleEndian.Uint16(buf[5:7])
		rt := binary.LittleEndian.Uint16(buf[7:9])
		state.LT = float32(lt) / 1023.0
		state.RT = float32(rt) / 1023.0
		lx := int16(binary.LittleEndian.Uint16(buf[9:11]))
		ly := int16(binary.LittleEndian.Uint16(buf[11:13]))
		rx := int16(binary.LittleEndian.Uint16(buf[13:15]))
		ry := int16(binary.LittleEndian.Uint16(buf[15:17]))
		state.LEFTX = float32(lx) / 32768.0
		state.LEFTY = float32(ly) / 32768.0
		state.RIGHTX = float32(rx) / 32768.0
		state.RIGHTY = float32(ry) / 32768.0

		const deadzone = 0.1
		if math.Abs(float64(state.LEFTX)) < deadzone {
			state.LEFTX = 0
		}
		if math.Abs(float64(state.LEFTY)) < deadzone {
			state.LEFTY = 0
		}
		if math.Abs(float64(state.RIGHTX)) < deadzone {
			state.RIGHTX = 0
		}
		if math.Abs(float64(state.RIGHTY)) < deadzone {
			state.RIGHTY = 0
		}

	case 0x07:
		if len(buf) >= 4 {
			state.GUIDE = buf[2]&0x01 != 0
		}
	}

	return state, nil
}

func setPollingFrequency(hz int) time.Duration {
	if hz <= 0 {
		return 16 * time.Millisecond
	}
	return time.Duration(1e9/hz) * time.Nanosecond
}

func logStateChanges(current, last *ControllerState) {
	if last == nil {
		return
	}

	val := reflect.ValueOf(*current)
	lastVal := reflect.ValueOf(*last)
	t := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := t.Field(i)

		if field.Type.Kind() != reflect.Bool || field.Name == "LastState" {
			continue
		}

		currentValue := val.Field(i).Bool()
		lastValue := lastVal.Field(i).Bool()

		if currentValue != lastValue {
			if currentValue {
				log.Printf("%s pressed", field.Name)
			} else {
				log.Printf("%s released", field.Name)
			}
		}
	}

	const analogThreshold = 0.1
	if math.Abs(float64(current.LEFTX-last.LEFTX)) > analogThreshold ||
		math.Abs(float64(current.LEFTY-last.LEFTY)) > analogThreshold {
		log.Printf("Left stick: %.2f, %.2f", current.LEFTX, current.LEFTY)
	}

	if math.Abs(float64(current.RIGHTX-last.RIGHTX)) > analogThreshold ||
		math.Abs(float64(current.RIGHTY-last.RIGHTY)) > analogThreshold {
		log.Printf("Right stick: %.2f, %.2f", current.RIGHTX, current.RIGHTY)
	}

	if math.Abs(float64(current.LT-last.LT)) > analogThreshold ||
		math.Abs(float64(current.RT-last.RT)) > analogThreshold {
		log.Printf("Triggers: LT=%.2f RT=%.2f", current.LT, current.RT)
	}
}

func main() {
	flag.Parse()

	controller, err := NewController()
	if err != nil {
		log.Fatalf("Failed to initialize controller: %v", err)
	}
	defer controller.Close()

	if err := controller.Initialize(); err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	sleepDuration := setPollingFrequency(*pollingFrequency)
	log.Printf("Polling frequency set to %d Hz", *pollingFrequency)
	log.Println("Xbox One controller connected and initialized")

	var lastState *ControllerState

	for {
		state, err := controller.ReadState()
		if err != nil {
			log.Printf("Read error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		logStateChanges(state, lastState)
		lastState = state
		time.Sleep(sleepDuration)
	}
}
