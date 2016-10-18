package gobot

import (
	"fmt"
	"log"
	"os"
	"os/signal"
)

// JSONRobot a JSON representation of a Robot.
type JSONRobot struct {
	Name        string            `json:"name"`
	Commands    []string          `json:"commands"`
	Connections []*JSONConnection `json:"connections"`
	Devices     []*JSONDevice     `json:"devices"`
}

// NewJSONRobot returns a JSONRobot given a Robot.
func NewJSONRobot(robot *Robot) *JSONRobot {
	jsonRobot := &JSONRobot{
		Name:        robot.Name,
		Commands:    []string{},
		Connections: []*JSONConnection{},
		Devices:     []*JSONDevice{},
	}

	for command := range robot.Commands() {
		jsonRobot.Commands = append(jsonRobot.Commands, command)
	}

	robot.Devices().Each(func(device Device) {
		jsonDevice := NewJSONDevice(device)
		jsonRobot.Connections = append(jsonRobot.Connections, NewJSONConnection(robot.Connection(jsonDevice.Connection)))
		jsonRobot.Devices = append(jsonRobot.Devices, jsonDevice)
	})
	return jsonRobot
}

// Robot is a named entity that manages a collection of connections and devices.
// It contains its own work routine and a collection of
// custom commands to control a robot remotely via the Gobot api.
type Robot struct {
	Name        string
	Work        func()
	connections *Connections
	devices     *Devices
	trap        func(chan os.Signal)
	AutoRun     bool
	done        chan bool
	Commander
	Eventer
}

// Robots is a collection of Robot
type Robots []*Robot

// Len returns the amount of Robots in the collection.
func (r *Robots) Len() int {
	return len(*r)
}

// Start calls the Start method of each Robot in the collection
func (r *Robots) Start(args ...interface{}) (errs []error) {
	autoRun := true
	if args[0] != nil {
		autoRun = args[0].(bool)
	}
	for _, robot := range *r {
		if errs = robot.Start(autoRun); len(errs) > 0 {
			for i, err := range errs {
				errs[i] = fmt.Errorf("Robot %q: %v", robot.Name, err)
			}
			return
		}
	}
	return
}

// Stop calls the Stop method of each Robot in the collection
func (r *Robots) Stop() (errs []error) {
	for _, robot := range *r {
		if errs = robot.Stop(); len(errs) > 0 {
			for i, err := range errs {
				errs[i] = fmt.Errorf("Robot %q: %v", robot.Name, err)
			}
			return
		}
	}
	return
}

// Each enumerates through the Robots and calls specified callback function.
func (r *Robots) Each(f func(*Robot)) {
	for _, robot := range *r {
		f(robot)
	}
}

// NewRobot returns a new Robot given optional accepts:
//
// 	[]Connection: Connections which are automatically started and stopped with the robot
//	[]Device: Devices which are automatically started and stopped with the robot
//	func(): The work routine the robot will execute once all devices and connections have been initialized and started
// A name will be automaically generated if no name is supplied.
func NewRobot(v ...interface{}) *Robot {
	r := &Robot{
		Name:        fmt.Sprintf("%X", Rand(int(^uint(0)>>1))),
		connections: &Connections{},
		devices:     &Devices{},
		done:        make(chan bool),
		trap: func(c chan os.Signal) {
			signal.Notify(c, os.Interrupt)
		},
		AutoRun:   true,
		Work:      nil,
		Eventer:   NewEventer(),
		Commander: NewCommander(),
	}

	for i := range v {
		switch v[i].(type) {
		case string:
			r.Name = v[i].(string)
		case []Connection:
			log.Println("Initializing connections...")
			for _, connection := range v[i].([]Connection) {
				c := r.AddConnection(connection)
				log.Println("Initializing connection", c.Name(), "...")
			}
		case []Device:
			log.Println("Initializing devices...")
			for _, device := range v[i].([]Device) {
				d := r.AddDevice(device)
				log.Println("Initializing device", d.Name(), "...")
			}
		case func():
			r.Work = v[i].(func())
		}
	}

	log.Println("Robot", r.Name, "initialized.")

	return r
}

// Start a Robot's Connections, Devices, and work.
func (r *Robot) Start(args ...interface{}) (errs []error) {
	if len(args) > 0 && args[0] != nil {
		r.AutoRun = args[0].(bool)
	}
	log.Println("Starting Robot", r.Name, "...")
	if cerrs := r.Connections().Start(); len(cerrs) > 0 {
		errs = append(errs, cerrs...)
		return
	}
	if derrs := r.Devices().Start(); len(derrs) > 0 {
		errs = append(errs, derrs...)
		return
	}
	if r.Work != nil {
		log.Println("Starting work...")
		go func() {
			r.Work()
			<-r.done
		}()
	}

	if r.AutoRun {
		c := make(chan os.Signal, 1)
		r.trap(c)
		if len(errs) > 0 {
			// there was an error during start, so we immediately pass the interrupt
			// in order to disconnect the initialized robots, connections and devices
			c <- os.Interrupt
		}

		// waiting for interrupt coming on the channel
		<-c

		// Stop calls the Stop method on itself, it we are "auto-running".
		r.Stop()
	}

	return
}

// Stop stops a Robot's connections and Devices
func (r *Robot) Stop() (errs []error) {
	log.Println("Stopping Robot", r.Name, "...")
	errs = append(errs, r.Devices().Halt()...)
	errs = append(errs, r.Connections().Finalize()...)
	r.done <- true
	return errs
}

// Devices returns all devices associated with this Robot.
func (r *Robot) Devices() *Devices {
	return r.devices
}

// AddDevice adds a new Device to the robots collection of devices. Returns the
// added device.
func (r *Robot) AddDevice(d Device) Device {
	*r.devices = append(*r.Devices(), d)
	return d
}

// Device returns a device given a name. Returns nil if the Device does not exist.
func (r *Robot) Device(name string) Device {
	if r == nil {
		return nil
	}
	for _, device := range *r.devices {
		if device.Name() == name {
			return device
		}
	}
	return nil
}

// Connections returns all connections associated with this robot.
func (r *Robot) Connections() *Connections {
	return r.connections
}

// AddConnection adds a new connection to the robots collection of connections.
// Returns the added connection.
func (r *Robot) AddConnection(c Connection) Connection {
	*r.connections = append(*r.Connections(), c)
	return c
}

// Connection returns a connection given a name. Returns nil if the Connection
// does not exist.
func (r *Robot) Connection(name string) Connection {
	if r == nil {
		return nil
	}
	for _, connection := range *r.connections {
		if connection.Name() == name {
			return connection
		}
	}
	return nil
}
