# DmService DB Exporter 达梦数据库Exporter


##### Table of Contents  

[Description](#Description)  
[Installation](#Installation)  
[Running](#Running)  
[Build](#Build)     
[Troubleshooting](#Troubleshooting)  


# Description

A [Prometheus](https://prometheus.io/) exporter for DmService copy after the Oracle exporter. 

The following metrics are exposed currently.

- dmdb_exporter_last_scrape_duration_seconds
- dmdb_exporter_last_scrape_error
- dmdb_exporter_scrapes_total
- dmdb_session_active
- dmdb_session_max
- dmdb_session_used
- dmdb_tablespace_free_percent
- dmdb_tablespace_free_space
- dmdb_tablespace_total_space
- dmdb_up

# Installation

## Docker

You can run via Docker using an existing image. 

```bash
docker run -d --name dmdb_exporter  -p 9161:9161 -e DATA_SOURCE_NAME=dm://SYSDBA:SYSDBA@localhost:5236?autoCommit=true ${image_name}
```



## Binary Release

Pre-compiled versions for Linux 64 bit  can be found under [releases].


# Running

Ensure that the environment variable DATA_SOURCE_NAME is set correctly before starting. For Example:

```bash
# using a complete url:
export DATA_SOURCE_NAME=dm://SYSDBA:SYSDBA@localhost:5236?autoCommit=true
# Then run the exporter
/path/to/binary/dmdb_exporter --log.level error  --default.metrics  /path/of/the/default-metrics.toml --web.listen-address 0.0.0.0:9161
```

# Integration with System D

Create file **/etc/systemd/system/dmdb_exporter.service** with the following content:

    [Unit]
    Description=Service for dm telemetry client
    After=network.target
    [Service]
    Environment=DATA_SOURCE_NAME=dm://SYSDBA:SYSDBA@localhost:5236?autoCommit=true
    ExecStart=/path/of/the/dmdb_exporter  --default.metrics  /path/of/the/default-metrics.toml --web.listen-address 0.0.0.0:9161
    [Install]
    WantedBy=multi-user.target

Then tell System D to read files:

    systemctl daemon-reload

Start this new service:

    systemctl start dmdb_exporter

Check service status:

    systemctl status dmdb_exporter

## Usage

```bash
usage: dmdb_exporter [<flags>]

Flags:
  -h, --help                     Show context-sensitive help (also try --help-long and --help-man).
      --web.listen-address=":9161"
                                 Address to listen on for web interface and telemetry. (env: LISTEN_ADDRESS)
      --web.telemetry-path="/metrics"
                                 Path under which to expose metrics. (env: TELEMETRY_PATH)
      --default.metrics="default-metrics.toml"
                                 File with default metrics in a TOML file. (env: DEFAULT_METRICS)
      --custom.metrics=""        File that may contain various custom metrics in a TOML file. (env: CUSTOM_METRICS)
      --query.timeout="5"        Query timeout (in seconds). (env: QUERY_TIMEOUT)
      --database.maxIdleConns=0  Number of maximum idle connections in the connection pool. (env: DATABASE_MAXIDLECONNS)
      --database.maxOpenConns=10
                                 Number of maximum open connections in the connection pool. (env: DATABASE_MAXOPENCONNS)
      --log.level="info"         Only log messages with the given severity or above. Valid levels: [debug, info, warn, error, fatal]
      --log.format="logger:stderr"
                                 Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true"
      --version                  Show application version.
```
