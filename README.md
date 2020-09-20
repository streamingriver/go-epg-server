# xmltv epg server

config.ini
```
listen = :8080 ; listen address
db = /path/to/file.db
xml = /path/to/xmltv.xml
index = /path/to/index.bleve
```

current program:

http://server/epg_js?aux_id=[auxid1999]&now=[unix timestamp]&limit=1

archive or feature 

http://server/epg_js?aux_id=[auxid19999]&start=[unix timestamp]&end=[unix timestamp]


kill -hup [pid] - to import file
