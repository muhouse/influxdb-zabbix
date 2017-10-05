Redesigned version (based on original work by @zensqlmonitor) where issues with installations using zabbix-proxy may occur - querying is based on timestamps and some data may be missing if arrived just a little late.

This version uses auto-increment values to track synced records, thus ensuring no data are missed. Only tested on PostgreSQL, but MySQL should work as well.

Auto-increment id column is not present in zabbix by default, so adding column "syncid" to respective tables needs to be done (instructions bellow).

# influxdb-zabbix

Gather data from Zabbix back-end and load to InfluxDB in near real-time for enhanced performance and easier usage with Grafana.

As InfluxDB provides an excellent compression rate (in our case: 7x), this project could be used also to archive Zabbix data.

## Getting Started

- InfluxDB:
	- [Install InfluxDB](https://docs.influxdata.com/influxdb/v1.3/introduction/installation/)
	- [Create a database with a retention period ](https://docs.influxdata.com/influxdb/v1.3/introduction/getting_started/) <br />
- Grafana:
	- [Install Grafana](http://docs.grafana.org/installation/)
	- [Using InfluxDB in Grafana](http://docs.grafana.org/features/datasources/influxdb/)
- influxdb-zabbix:
	- [Install GO](https://golang.org/doc/install)
	- [Setup you GOPATH](https://golang.org/doc/code.html#GOPATH)
	- Run ``` go get github.com/vasekch/influxdb-zabbix ```
	- Edit influxdb-zabbix.conf to match your needs  <br />
- PostgreSQL:

	Create user:
	```SQL
	CREATE USER influxdb_zabbix WITH PASSWORD '***';
	GRANT USAGE ON SCHEMA public TO influxdb_zabbix;
	```
	Grants at the database level:
	```SQL
	GRANT SELECT ON public.history, public.history_uint TO influxdb_zabbix;
	GRANT SELECT ON public.trends, public.trends_uint TO influxdb_zabbix;
	```

	Create auto-increment column:
	```SQL
	ALTER TABLE public.history ADD COLUMN syncid SERIAL;
	ALTER TABLE public.history_uint ADD COLUMN syncid SERIAL;
	ALTER TABLE public.trends ADD COLUMN syncid SERIAL;
	ALTER TABLE public.trends_uint ADD COLUMN syncid SERIAL;
	```

- MariaDB / MySQL:

	Create user:
	```SQL
	CREATE USER 'influxdb_zabbix'@'localhost' IDENTIFIED BY '***';
	```

	Grants at the database level:
	```SQL
	GRANT SELECT ON zabbix.trends TO influxdb_zabbix@localhost;
	GRANT SELECT ON zabbix.trends_uint TO influxdb_zabbix@localhost;
	GRANT SELECT ON zabbix.history TO influxdb_zabbix@localhost;
	GRANT SELECT ON zabbix.history_uint TO influxdb_zabbix@localhost;
 	flush privileges;
	```

	Create indexes (not tested):
	```SQL
	ALTER TABLE `history` ADD `syncid` INT NOT NULL AUTO_INCREMENT;
	ALTER TABLE `history_uint` ADD `syncid` INT NOT NULL AUTO_INCREMENT;
	ALTER TABLE `trends` ADD `syncid` INT NOT NULL AUTO_INCREMENT;
	ALTER TABLE `trends_uint` ADD `syncid` INT NOT NULL AUTO_INCREMENT;
	```

### How to use GO code

- Run in background: ``` go run influxdb-zabbix.go & ```
- Build in the current directory: ``` go build influxdb-zabbix.go ```
- Install in $GOPATH/bin: ``` go install influxdb-zabbix.go ```

### Goodies
Have a look to the scripts folder

### Dependencies
- Go 1.7+
- TOML parser (https://github.com/BurntSushi/toml)
- Pure Go Postgres driver for database/sql (https://github.com/lib/pq/)
- Pure Go MySQL driver for database/sql (https://github.com/go-sql-driver/mysql/)

## Configuration: influxdb-zabbix.conf

- PostgreSQL and MariaDB/MySQL supported.

- Tables that can be replicated are:
  - history
  - history_uint
  - trends
  - trends_uint
- Tables like history_log, _text and _str are not replicated.

- Configurable at table-level:
  - interval: polling interval, minimum of 15 sec
  - records per batch : number of records/batch to extract from zabbix backend (actual number of records reported may differ, because zabbix rows are repeated for different host groups)
  - output rows per batch :  allow the destination load to be splitted in multiple batches

## License

MIT-LICENSE. See LICENSE file provided in the repository for details
