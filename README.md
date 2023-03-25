# xmltv epg server

config.ini
```
listen = :8080 ; listen address
db = /path/to/file.db
xml = /path/to/xmltv.xml
index = /path/to/index.bleve
url = http://url/to/xml
token = t0psecr3t
```

current program:

http://server/epg_js?aux_id=[auxid1999]&now=[unix timestamp]&limit=1

archive or feature 

http://server/epg_js?aux_id=[auxid19999]&start=[unix timestamp]&end=[unix timestamp]


kill -hup [pid] - to import file


if url is set - fetch and parse xml

curl -H "X-Token: t0psecr3t" http://server/-/reload 
