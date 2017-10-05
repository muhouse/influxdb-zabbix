package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	cfg "github.com/vasekch/influxdb-zabbix/config"
	helpers "github.com/vasekch/influxdb-zabbix/helpers"
	input "github.com/vasekch/influxdb-zabbix/input"
	log "github.com/vasekch/influxdb-zabbix/log"
	influx "github.com/vasekch/influxdb-zabbix/output/influxdb"
	registry "github.com/vasekch/influxdb-zabbix/reg"
)

var m runtime.MemStats

var exitChan = make(chan int)

var wg sync.WaitGroup

type TOMLConfig cfg.TOMLConfig

var config cfg.TOMLConfig

type DynMap map[string]interface{}

type Param struct {
	input  Input
	output Output
}

type Input struct {
	provider          string
	address           string
	tablename         string
	interval          int
	inputrowsperbatch int
}

type Output struct {
	address            string
	database           string
	username           string
	password           string
	precision          string
	outputrowsperbatch int
}

type InfluxDB struct {
	output Output
}

var mapTables = make(registry.MapTable)

//
// Gather data
//
func (p *Param) gatherData() error {

	var infoLogs []string
	var currTable string = p.input.tablename
	var currTableForLog string = helpers.RightPad(currTable, " ", 12-len(currTable))

	// read registry
	if err := registry.Read(&config, &mapTables); err != nil {
		fmt.Println(err)
		return err
	}

	// set start/end id
	startid := registry.GetValueFromKey(mapTables, currTable)
	var endid int = startid + p.input.inputrowsperbatch

	//
	// <--  Extract
	//
	var tlen int = len(currTable)
	infoLogs = append(infoLogs,
		fmt.Sprintf(
			"----------- | %s | [id %v --> id %v]",
			currTableForLog,
			startid,
			endid))

	//start watcher
	startwatch := time.Now()
	ext := input.NewExtracter(
		p.input.provider,
		p.input.address,
		currTable,
		startid,
		endid)

	if err := ext.Extract(); err != nil {
		log.Error(1, "Error while executing script: %s", err)
		return err
	}

	// count rows
	var rowcount int = len(ext.Result)
	infoLogs = append(infoLogs,
		fmt.Sprintf(
			"<-- Extract | %s | %v rows in %s",
			currTableForLog,
			rowcount,
			time.Since(startwatch)))

	// set max id
	var maxid = startid
	if ext.Maxid > 0 {
		maxid = ext.Maxid
	// 	// debug print
	// 	infoLogs = append(infoLogs,
	// 		fmt.Sprintf(
	// 			"--- Debug   | %s | Updating maxid to %v",
	// 			currTableForLog,
	// 			maxid))
	// } else {
	// 	// debug print
	// 	infoLogs = append(infoLogs,
	// 		fmt.Sprintf(
	// 			"--- Debug   | %s | Maxid remains %v",
	// 			currTableForLog,
	// 			maxid))
	}


	// no row
	if rowcount == 0 {
		infoLogs = append(infoLogs,
			fmt.Sprintf(
				"--> Load    | %s | No data",
				currTableForLog))
	} else {
		//
		// --> Load
		//
		startwatch = time.Now()
		inlineData := ""

		if rowcount <= p.output.outputrowsperbatch {

			inlineData = strings.Join(ext.Result[:], "\n")

			loa := influx.NewLoader(
				fmt.Sprintf(
					"%s/write?db=%s&precision=%s",
					p.output.address,
					p.output.database,
					p.output.precision),
				p.output.username,
				p.output.password,
				inlineData)

			if err := loa.Load(); err != nil {
				log.Error(1, "Error while loading data for %s. %s", currTable, err)
				return err
			}

			infoLogs = append(infoLogs,
				fmt.Sprintf(
					"--> Load    | %s | %v rows in %s",
					currTableForLog,
					rowcount,
					time.Since(startwatch)))

		} else { // else split result in multiple batches

			var batches float64 = float64(rowcount) / float64(p.output.outputrowsperbatch)
			var batchesCeiled float64 = math.Ceil(batches)
			var batchLoops int = 1
			var minRange int = 0
			var maxRange int = 0

			for batches > 0 { // while
				if batchLoops == 1 {
					minRange = 0
				} else {
					minRange = maxRange
				}

				maxRange = batchLoops * p.output.outputrowsperbatch
				if maxRange >= rowcount {
					maxRange = rowcount
				}

				// create slide
				datapart := []string{}
				for i := minRange; i < maxRange; i++ {
					datapart = append(datapart, ext.Result[i])
				}

				inlineData = strings.Join(datapart[:], "\n")

				startwatch = time.Now()
				loa := influx.NewLoader(
					fmt.Sprintf(
						"%s/write?db=%s&precision=%s",
						p.output.address,
						p.output.database,
						p.output.precision),
					p.output.username,
					p.output.password,
					inlineData)

				if err := loa.Load(); err != nil {
					log.Error(1, "Error while loading data for %s. %s", currTable, err)
					return err
				}

				// log
				tableBatchName := fmt.Sprintf("%s (%v/%v)",
					currTable,
					batchLoops,
					batchesCeiled)

				tlen = len(tableBatchName)

				infoLogs = append(infoLogs,
					fmt.Sprintf("--> Load    | %s | %v rows in %s",
						helpers.RightPad(tableBatchName, " ", 13-tlen),
						len(datapart),
						time.Since(startwatch)))

				batchLoops += 1
				batches -= 1

			} // end while
		}
	}

	// Save in registry
	registry.Save(config,
		currTable,
		maxid)


	tlen = len(currTable)
	infoLogs = append(infoLogs,
		fmt.Sprintf("--- Waiting | %s | %v sec ",
			currTableForLog,
			p.input.interval))

	if config.Logging.LevelFile == "Trace" || config.Logging.LevelConsole == "Trace" {
		runtime.ReadMemStats(&m)
		log.Trace(fmt.Sprintf("--- Memory usage: Alloc = %s | TotalAlloc = %s | Sys = %s | NumGC = %v",
			helpers.IBytes(m.Alloc / 1024),
			helpers.IBytes(m.TotalAlloc / 1024),
			helpers.IBytes(m.Sys / 1024),
			m.NumGC))
	}


	// print all log messages
	print(infoLogs)

	return nil
}

//
// Print all messages
//
func print(infoLogs []string) {
	for i := 0; i < len(infoLogs); i++ {
		log.Info(infoLogs[i])
	}
}

//
// Gather data loop
//
func (p *Param) gather() error {
	for {
		err := p.gatherData()
		if err != nil {
			return err
		}

		time.Sleep(time.Duration(p.input.interval) * time.Second)
	}
	return nil
}

//
// Init
//
func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

//
//  Read TOML configuration
//
func readConfig() {

	// command-line flag parsing
	flag.Parse()

	// read configuration file
	if err := cfg.Parse(&config); err != nil {
		fmt.Println(err)
		return
	}
	// validate configuration file
	if err := cfg.Validate(&config); err != nil {
		fmt.Println(err)
		return
	}
}

//
// Read registry file
//
func readRegistry() {
	if err := registry.Read(&config, &mapTables); err != nil {
		log.Error(0, err.Error())
		return
	}
}

//
// Init global logging
//
func initLog() {
	log.Init(config)
}

//
// Listen to System Signals
//
func listenToSystemSignals() {
	signalChan := make(chan os.Signal, 1)
	code := 0

	signal.Notify(signalChan, os.Interrupt)
	signal.Notify(signalChan, os.Kill)
	signal.Notify(signalChan, syscall.SIGTERM)

	select {
	case sig := <-signalChan:
		log.Info("Received signal %s. shutting down", sig)
	case code = <-exitChan:
		switch code {
		case 0:
			log.Info("Shutting down")
		default:
			log.Warn("Shutting down")
		}
	}
	log.Close()
	os.Exit(code)
}

//
// Main
//
func main() {

	log.Info("***** Starting influxdb-zabbix *****")

	// listen to System Signals
	go listenToSystemSignals()

	readConfig()
	readRegistry()
	initLog()

	// set of active tables
	log.Trace("--- Active tables:")
	var tables = []*cfg.Table{}
	for _, table := range config.Tables {
		if table.Active {
			var tlen int = len(table.Name)

			log.Trace(
				fmt.Sprintf(
					"----------- | %s | Each %v sec | Input %v records per batch | Output by %v",
					helpers.RightPad(table.Name, " ", 12-tlen),
					table.Interval,
					table.Inputrowsperbatch,
					table.Outputrowsperbatch))

			tables = append(tables, table)
		}
	}

	log.Info("--- Start polling")

	var provider string = (reflect.ValueOf(config.Zabbix).MapKeys())[0].String()
	var address string = config.Zabbix[provider].Address
	log.Trace(fmt.Sprintf("--- Provider: %s", provider))

	influxdb := config.InfluxDB

	for _, table := range tables {

		input := Input{
			provider,
			address,
			table.Name,
			table.Interval,
			table.Inputrowsperbatch}

		output := Output{
			influxdb.Url,
			influxdb.Database,
			influxdb.Username,
			influxdb.Password,
			influxdb.Precision,
			table.Outputrowsperbatch}

		p := &Param{input, output}

		wg.Add(1)
		go p.gather()
	}
	wg.Wait()
}
