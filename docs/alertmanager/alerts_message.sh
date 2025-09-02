#!/usr/bin/env bash
alerts1='[
  {
    "labels": {
       "alertname": "DiskRunningFull",
       "dev": "sda1",
       "instance": "example1"
     },
     "annotations": {
        "info": "The disk sda1 is running full",
        "summary": "please check the instance example1"
      }
  }
  ]'


curl -XPOST -H "Content-Type: application/json" -d "$alerts1" http://127.0.0.1:9093/api/v2/alerts
# 老版本v1接口
#curl -XPOST -d"$alerts1" http://localhost:9093/api/v1/alerts
