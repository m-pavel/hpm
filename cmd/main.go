package main

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const bluezPath = "/org/bluez/hci0"

type childNode struct {
	Name string `xml:"name,attr"`
}

type Node struct {
	XMLName xml.Name    `xml:"node"`
	Nodes   []childNode `xml:"node"`
}

type Device struct {
	name       string
	percentage int
	address    string
	connected  bool
	paired     bool
}

var rootCmd = &cobra.Command{
	Use:   "list",
	Short: "List tilesets",
	RunE: func(cmd *cobra.Command, args []string) error {
		battery := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "battery_level",
			Help: "Battery level",
		})
		connected := prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "connected",
			Help: "Device level",
		})
		conn, err := dbus.ConnectSystemBus()
		if err != nil {
			return err
		}

		addr := viper.GetString("device")
		pep := viper.GetString("pep")
		pj := viper.GetString("pj")
		interval := viper.GetInt("interval")

		log.Printf("Interval: %d", interval)
		log.Printf("Push gateway endpoint: %s", pep)

		d := GetDeviceByAddress(conn, addr)
		if d == nil {
			return fmt.Errorf("no device found '%s'", addr)
		}

		pusher1 := push.New(pep, pj).
			Collector(battery).
			Collector(connected).
			Grouping("address", addr).
			Client(&http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
				},
			})
		lost := true
		for {
			di := GetDevice(conn, *d)
			battery.Set(float64(di.percentage))
			if di.connected {
				if lost {
					log.Println("Connected")
					lost = false
				}
				connected.Set(1)
			} else {
				if !lost {
					log.Println("Disconnected")
					lost = true
				}
				connected.Set(0)
				battery.Set(0)
			}
			if err = pusher1.Push(); err != nil {
				log.Println(err)
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
		return nil
	},
}

func main() {
	rootCmd.Flags().String("pep", "http://localhost:9091/", "Prometheus pushgateway endpoint")
	rootCmd.Flags().String("pj", "battery", "Prometheus job")
	rootCmd.Flags().String("device", "00:00:00:00:00:00", "Device BT MAC")
	rootCmd.Flags().Int("interval", 30, "Interval seconds")
	viper.BindPFlags(rootCmd.Flags())
	viper.BindEnv()
	viper.AutomaticEnv()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func GetDeviceByAddress(conn *dbus.Conn, address string) *dbus.ObjectPath {
	devices := SearchAll(conn)
	for _, d := range devices {
		dd := GetDevice(conn, d)
		if dd.address == address {
			return &d
		}
	}
	return nil
}

func GetDevice(bus *dbus.Conn, obj dbus.ObjectPath) Device {
	var info map[string]dbus.Variant
	bus.Object("org.bluez", obj).Call("org.freedesktop.DBus.Properties.GetAll", 0, "org.bluez.Device1").Store(&info)

	var connec, pair bool
	info["Connected"].Store(&connec)
	info["Paired"].Store(&pair)

	perc := -1
	if pair {
		bat, _ := bus.Object("org.bluez", obj).GetProperty("org.bluez.Battery1.Percentage")

		// if device is paired, but battery level can't be read, set percentage to -1
		if bat.Value() != nil {
			bat.Store(&perc)
		}
	}

	var name, addr string = "", ""
	if info["Name"].Value() != nil {
		info["Name"].Store(&name)
	}
	if info["Address"].Value() != nil {
		info["Address"].Store(&addr)
	}

	return Device{name, perc, addr, connec, pair}

}

func SearchAll(bus *dbus.Conn) []dbus.ObjectPath {
	var introspect string
	bus.Object("org.bluez", bluezPath).Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&introspect)

	var devices Node
	err := xml.Unmarshal([]byte(introspect), &devices)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var l []dbus.ObjectPath
	for _, d := range devices.Nodes {
		l = append(l, dbus.ObjectPath(bluezPath+"/"+d.Name))
	}
	return l
}
