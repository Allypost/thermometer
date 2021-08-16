package main

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/boz/go-throttle"
	"github.com/go-ble/ble"
	"github.com/go-ble/ble/examples/lib/dev"
	_ "github.com/lib/pq"
)

const (
	host     = "enceladus"
	port     = 5432
	user     = "thermometer"
	password = "zA3zhJwFvLmVtZNgxX68SEQomg78G96AeHxx2nHhrMCTM4GtEo9iGKbr0Xi1i8RB"
	dbname   = "thermometer"
)

var PSQL_CONN = fmt.Sprintf(
	"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
	host,
	port,
	user,
	password,
	dbname,
)

const THERMOMETER_ADDRESS_PREFIX = "a4:c1:38:"

type ThermometerMeasurement struct {
	Address        string
	Time           time.Time
	Temperature    int16
	Humidity       uint8
	BatteryPercent uint8
}

func (oldMeasurement *ThermometerMeasurement) Equals(newMeasurement ThermometerMeasurement) bool {
	return oldMeasurement.Temperature == newMeasurement.Temperature &&
		oldMeasurement.Humidity == newMeasurement.Humidity
}

type Measurements struct {
	lock   sync.RWMutex
	keys   []string
	values map[string]ThermometerMeasurement
}

func (m *Measurements) New() Measurements {
	return Measurements{
		keys:   make([]string, 0),
		values: map[string]ThermometerMeasurement{},
		lock:   sync.RWMutex{},
	}
}

func (m *Measurements) Get(address string) (ThermometerMeasurement, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	val, ok := m.values[address]

	return val, ok
}

func (m *Measurements) Changed(newValue ThermometerMeasurement) bool {
	oldValue, exists := m.Get(newValue.Address)

	if !exists {
		return true
	}

	return !oldValue.Equals(newValue)
}

func (m *Measurements) Set(measurement ThermometerMeasurement) {
	m.lock.Lock()
	defer m.lock.Unlock()

	address := measurement.Address

	if _, ok := m.values[address]; !ok {
		m.keys = append(m.keys, address)
		sort.Strings(m.keys)
	}

	m.values[address] = measurement
}

func (m *Measurements) Update(measurement ThermometerMeasurement) bool {
	if !m.Changed(measurement) {
		return false
	}

	m.Set(measurement)

	insertMeasurement(measurement)

	return true
}

func (m *Measurements) Keys() []string {
	return m.keys
}

var measurements = Measurements{
	keys:   make([]string, 0),
	values: map[string]ThermometerMeasurement{},
	lock:   sync.RWMutex{},
}
var throttledPrintMeasurements = throttle.ThrottleFunc(1*time.Second, true, printMeasurements)
var db *sql.DB

func main() {
	d, err := dev.NewDevice("default")
	if err != nil {
		panic("can't new device")
	}
	ble.SetDefaultDevice(d)

	err = setupDb()
	checkError(err)
	defer db.Close()

	err = ble.Scan(context.Background(), true, handleScanEvent, nil)
	checkError(err)
}

func checkError(err error) {
	if err != nil {
		println(err.Error())
		panic(err)
	}
}

func handleScanEvent(advertisement ble.Advertisement) {
	address := advertisement.Addr().String()

	if !strings.HasPrefix(address, THERMOMETER_ADDRESS_PREFIX) {
		return
	}

	serviceData := advertisement.ServiceData()

	for id := range serviceData {
		if serviceData[id].UUID.Equal(ble.UUID16(0x181a)) {
			updateThermometer(address, serviceData[id].Data)
			break
		}
	}

	throttledPrintMeasurements.Trigger()
}

func updateThermometer(Address string, thermometerData []byte) {
	var Time = time.Now()
	var Temperature int16 = int16(binary.BigEndian.Uint16(thermometerData[6:8]))
	var Humidity uint8 = thermometerData[8]
	var BatteryPercent uint8 = thermometerData[9]

	newMeasurement := ThermometerMeasurement{
		Address,
		Time,
		Temperature,
		Humidity,
		BatteryPercent,
	}

	measurements.Update(newMeasurement)
}

func setupDb() (err error) {
	db, err = sql.Open("postgres", PSQL_CONN)
	if err != nil {
		return err
	}

	createSql := `
	create table if not exists measurements
	(
			id          serial                                             not null
					constraint measurements_pk
							primary key,
			address     varchar(32)                                        not null,
			temperature smallint                                           not null,
			humidity    smallint                                           not null,
			measured_at timestamp with time zone default CURRENT_TIMESTAMP not null
	);
	
	create index if not exists measurements_address_index
			on measurements (address);
	`

	_, err = db.Exec(createSql)
	return err
}

func insertMeasurement(measurement ThermometerMeasurement) {
	db.Exec(
		`insert into "measurements" ("address", "temperature", "humidity") values ($1, $2, $3)`,
		measurement.Address,
		measurement.Temperature,
		measurement.Humidity,
	)
}

func printMeasurements() {
	log.SetFlags(0)

	// Clear screen and move to 1,1
	log.Printf("\033[2J\033[%d;%dH", 1, 1)

	log.Printf("%s\n\n", time.Now())
	log.Println("--------------------")
	for _, address := range measurements.Keys() {
		thermometer, _ := measurements.Get(address)
		log.Printf("     Address: %s\n", address)
		log.Printf("          At: %s\n", thermometer.Time)
		log.Printf(" Temperature: %.1f°C\n", float32(thermometer.Temperature)/10)
		log.Printf("    Humidity: %d%%\n", thermometer.Humidity)
		log.Printf("     Battery: %d%%\n", thermometer.BatteryPercent)
		log.Println("--------------------")
	}
}
