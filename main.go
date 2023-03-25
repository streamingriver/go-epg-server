package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/boltdb/bolt"
	"github.com/docopt/docopt-go"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
)

var cfg *ini.File

const VERSION = `0.1`

const usage = `
Usage:
    go-epg-json --config=<config> [--debug-sql] [now]

Options:
    -h --help           Show this screen
    --config=<config>   Config file [default: /etc/go-epg-json.ini]
    --debug-sql         Log quries [default: false]
`

func init() {
	runtime.GOMAXPROCS(4)
}

var db *bolt.DB
var blvidx bleve.Index
var xmlURL string = ""
var xmlFile string

func main() {

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	go func() {
		for sig := range c {
			log.Printf("signal: %v", sig)
			ImportXML()
		}
	}()

	log.Printf("pid [%v]", syscall.Getpid())

	args, _ := docopt.Parse(usage, nil, true, VERSION, true)

	cfg, _ = ini.Load(args["--config"])

	xmlFile = cfg.Section("").Key("xml").String()
	xmlURL = cfg.Section("").Key("url").String()

	// blvidx, _ = bleve.NewMemOnly(bleve.NewIndexMapping())

	v := cfg.Section("").Key("db").String()
	var err error
	_, _ = v, err
	blv := bleve.NewIndexMapping()
	idx := cfg.Section("").Key("index").MustString("./index.bleve")
	blvidx, err = bleve.New(idx, blv)
	if err != nil {
		blvidx, _ = bleve.Open(idx)
	}

	db, err = bolt.Open(v, 0600, nil)
	if err != nil {
		log.Fatalf("bolt open error: %v", err)
	}
	dbName := CurrentDB()
	if dbName == "" {
		if err = ImportXML(); err != nil {
			log.Fatalln(err)
		}
	}
	dbName = CurrentDB()
	log.Printf("Using with '%s' db", dbName)

	router := mux.NewRouter()
	router.HandleFunc("/short-epg.json", AllChannelsShortEpg)

	router.HandleFunc("/epg_js", HandleEpg)

	router.HandleFunc("/search", SearchHandler)

	router.HandleFunc("/_health", HealthHandler)

	router.HandleFunc("/tv/tvip/tvipapi/json/epg/{auxID}/{date}.json", func(w http.ResponseWriter, r *http.Request) {

	})
	router.HandleFunc("/tv/tvip/tvipapi/json/short_epg/{token}.json", func(w http.ResponseWriter, r *http.Request) {

	})

	router.HandleFunc("/ep/epg/{auxID}/epg/pf.json", func(w http.ResponseWriter, r *http.Request) {
		// strip 4 numbers from beggining
	})
	router.HandleFunc("/ep/epg/{auxID}/epg/day.json", func(w http.ResponseWriter, r *http.Request) {
		// strip 4 numbers from beggining
	})
	router.HandleFunc("/ep/epg/{auxID}/epg/archive.json", func(w http.ResponseWriter, r *http.Request) {
		// strip 4 numbers from beggining
	})

	router.HandleFunc("/-/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != cfg.Section("").Key("token").String() {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		go func() {
			ImportXML()
		}()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	l := cfg.Section("").Key("listen").String()
	log.Fatal(http.ListenAndServe(l, router))

}

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	dbName := CurrentDB()
	_ = dbName
	query := bleve.NewMatchQuery(q)
	search := bleve.NewSearchRequest(query)
	results, _ := blvidx.Search(search)

	response := Programs{}

	for _, result := range results.Hits {
		parts := strings.Split(result.ID, ":")
		_key := []byte(parts[0])
		_bucket := []byte(parts[1])
		db.View(func(tx *bolt.Tx) error {
			rootBucket := tx.Bucket([]byte(dbName))
			bucket := rootBucket.Bucket(_bucket)
			b := bucket.Get(_key)
			var p Program
			json.Unmarshal(b, &p)
			response = append(response, p)
			return nil
		})

	}
	json.NewEncoder(w).Encode(response)

}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func HandleEpg(w http.ResponseWriter, r *http.Request) {
	ip := getClientIp(r)
	log.Printf("[%s] %s", ip, r.URL.String())

	limit := r.URL.Query().Get("limit")
	aux_id := r.URL.Query().Get("aux_id")
	_ = aux_id

	dbName := CurrentDB()
	_ = dbName
	if len(limit) == 0 {

		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")

		_, _ = start, end
		var programs Programs

		db.View(func(tx *bolt.Tx) error {
			rootBucket := tx.Bucket([]byte(dbName))
			bucket := rootBucket.Bucket([]byte(aux_id))
			if bucket == nil {
				return errors.New("Bucket doesnt exists, " + aux_id)
			}
			c := bucket.Cursor()
			intStart, _ := strconv.Atoi(start)
			k, v := c.Seek(itob(int(intStart)))
			if !bytes.Equal(k, itob(intStart)) {
				k, v = c.Prev()
			}

			intEnd, _ := strconv.Atoi(end)
			max := itob(int(intEnd))

			for k, v = c.Seek(k); k != nil && bytes.Compare(k, max) <= 0; k, v = c.Next() {
				var program Program

				json.Unmarshal(v, &program)

				programs = append(programs, program)
			}
			_ = v

			return nil
		})
		w.Write(encode(programs))
	} else {
		now := r.URL.Query().Get("now")
		var program Program
		err := db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte(dbName)).Bucket([]byte(aux_id))

			_ = bucket
			if bucket == nil {
				return errors.New("Bucket doesnt exists, " + aux_id)
			}
			c := bucket.Cursor()
			tnow, _ := strconv.Atoi(now)
			k, v := c.Seek(itob(int(tnow)))
			if !bytes.Equal(k, []byte(now)) {
				k, v = c.Prev()
			}
			_ = v

			json.Unmarshal(v, &program)
			return nil
		})
		if err != nil {
			log.Printf("%v", err)
		}
		_ = now
		w.Write(encode(program))
	}

}

func encode(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

type Program struct {
	Title           string `json:"title"`
	TitleLang       string `json:"title_l"`
	Rating          int    `json:"rating"`
	End             int    `json:"end"`
	Start           int    `json:"start"`
	Description     string `json:"descr"`
	PrId            string `json:"pr_id"`
	DescriptionLang string `json:"descr_l"`
	CategoryId      int    `json:"cat_id"`
	AuxId           string `json:"aux_id"`
	Icon            string `json:"icon"`
}

type Programs []Program

func getClientIp(r *http.Request) string {
	remote_addr := r.RemoteAddr
	idx := strings.LastIndex(remote_addr, ":")
	if idx != -1 {
		remote_addr = remote_addr[0:idx]
		if remote_addr[0] == '[' && remote_addr[len(remote_addr)-1] == ']' {
			remote_addr = remote_addr[1 : len(remote_addr)-1]
		}
	}
	if r.Header.Get("X-Forwarded-for") != "" {
		remote_addr = r.Header.Get("X-Forwarded-for")
	}
	if r.Header.Get("X-Real-IP") != "" && remote_addr == "127.0.0.1" {
		remote_addr = r.Header.Get("X-Real-IP")
	}
	return remote_addr
}

// itob returns an 8-byte big endian representation of v.
func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func CurrentDB() string {
	var dbName []byte
	db.Update(func(tx *bolt.Tx) error {
		bucket, _ := tx.CreateBucketIfNotExists([]byte("system"))
		dbName = bucket.Get([]byte("db-name"))
		return nil
	})
	return string(dbName)
}

func ImportXML() error {
	log.Printf("%v", xmlURL)
	if xmlURL != "" {
		log.Println("Downloading XML")
		err := fetchXML()
		if err != nil {
			return err
		}
	}

	log.Println("Starting ImportXML")
	var err error

	err = blvidx.Close()

	if err != nil {
		println(err.Error())
	}
	idx := cfg.Section("").Key("index").MustString("./index.bleve")

	err = os.RemoveAll(idx)
	if err != nil {
		panic(err)
	}

	blvidx, err = bleve.Open(idx)
	if err != nil {
		println(err.Error())
		blv := bleve.NewIndexMapping()
		blvidx, _ = bleve.New(idx, blv)
	}

	var xmlPrograms vProgramsQuery
	field := []byte("db-name")
	current := []byte("")

	fi, err := os.Stat(xmlFile)
	if err != nil {
		log.Printf("XML file doesnt exists, downloading from %s", xmlURL)
		err = fetchXML()
		if err != nil {
			log.Fatalf("XML download error: %v", err)
		}
	}
	_ = fi

	log.Println("ImportXML opening xml file")
	file, err := os.Open(xmlFile)
	if err != nil {
		log.Printf("Open XML file error: %v", err)
		return err
	}

	log.Println("ImportXML parsing xml file")
	err = xml.NewDecoder(file).Decode(&xmlPrograms)
	if err != nil {
		fmt.Printf("xml decode error: %v", err)
		return err
	}

	log.Println("ImportXML writing data to db")
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, _ := tx.CreateBucketIfNotExists([]byte("system"))

		dbName := bucket.Get(field)
		if bytes.Equal(dbName, []byte("programs1")) {
			current = []byte("programs2")
		} else {
			current = []byte("programs1")
		}

		log.Printf("Importing to %s bucket", current)

		tx.CreateBucketIfNotExists(current)

		if err = tx.DeleteBucket(current); err != nil {
			return errors.Wrap(err, "ImportXML error while deleting bucked")
		}
		if _, err = tx.CreateBucket(current); err != nil {
			return errors.Wrap(err, "ImportXML error while createing bucket")
		}

		return nil
	})
	if err != nil {
		log.Printf("ImportXML step 1 error: %v", err)
		return err
	}

	trx, _ := db.Begin(true)
	batch := blvidx.NewBatch()
	n := 0
	for _, program := range xmlPrograms.Programs {
		rootBucket, _ := trx.CreateBucketIfNotExists(current)
		AuxIdBucket, _ := rootBucket.CreateBucketIfNotExists([]byte(program.ChannelId))

		_key := itob(int(program.Start.Unix()))

		var prog Program
		prog.AuxId = program.ChannelId
		prog.Start = int(program.Start.Unix())
		prog.End = int(program.Stop.Unix())
		prog.Title = program.Title
		prog.Description = program.Description

		var sp SearchProgram
		sp.Title = prog.Title
		sp.Description = prog.Description

		var s string
		s = string(_key) + ":" + program.ChannelId + ":" + program.Title
		_ = s

		batch.Index(s, sp.Title)

		_value, _ := json.Marshal(prog)

		AuxIdBucket.Put(_key, _value)

		n++
		if n%10000 == 0 {
			log.Printf("Progress... %v", n)
			err = blvidx.Batch(batch)
			batch = blvidx.NewBatch()
			if err != nil {
				log.Printf("%v", err)

			}
			// batch = blvidx.NewBatch()
			trx.Commit()
			trx, _ = db.Begin(true)
		}
	}
	blvidx.Batch(batch)
	trx.Commit()

	log.Println("Switching database to ", string(current))

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("system"))
		if err != nil {
			return errors.Wrapf(err, "ImportXML bucket not found")
		}
		bucket.Put(field, current)
		return nil
	})
	if err != nil {
		log.Printf("ImportXML step 2 error: %v", err)
		return err
	}
	return nil
}

type vProgramsQuery struct {
	Programs []vProgram `xml:"programme"`
}

type timeField struct {
	time.Time
}

func (tf *timeField) UnmarshalXML(d *xml.Decoder, e xml.StartElement) error {
	fmt.Printf("%v", e.Attr)
	var st string
	d.DecodeElement(&st, &e)
	t := fmt.Sprintf("%s %s:%s:%s %s", st[:8], st[8:10], st[10:12], st[12:14], st[15:20])
	p, err := time.Parse("20060102 15:04:05 -0700", t)
	if err != nil {
		return err
	}
	*tf = timeField{p}
	return nil
}
func (tf *timeField) UnmarshalXMLAttr(e xml.Attr) error {
	st := e.Value
	t := fmt.Sprintf("%s %s:%s:%s %s", st[:8], st[8:10], st[10:12], st[12:14], st[15:20])
	p, err := time.Parse("20060102 15:04:05 -0700", t)
	if err != nil {
		return err
	}
	*tf = timeField{p}
	return nil
}

type vProgram struct {
	Start       timeField `xml:"start,attr"`
	Stop        timeField `xml:"stop,attr"`
	ChannelId   string    `xml:"channel,attr"`
	Title       string    `xml:"title"`
	Date        string    `xml:"date"`
	Description string    `xml:"desc"`
	Genre       string    `xml:"genre"`
}

type SearchProgram struct {
	Title       string
	Description string
}

type ProgramEvent struct {
	Start        int     `json:"start"`
	End          int     `json:"end"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	age_group_id *string `json:"age_group_id"`
}

type ProgramShort struct {
	ChannelID string `json:"channel_id"`
	Events    []ProgramEvent
}

type Response struct {
	Method   string      `json:"method"`
	Status   int         `json:"status"`
	Text     *string     `json:"text"`
	Response interface{} `json:"response"`
}

type Channels struct {
	Version  int64          `json:"version"`
	Channels []ProgramShort `json:"channels"`
}

func AllChannelsShortEpg(w http.ResponseWriter, r *http.Request) {
	ip := getClientIp(r)
	log.Printf("[%s] %s", ip, r.URL.String())

	dbName := CurrentDB()
	_ = dbName
	now := fmt.Sprintf("%d", time.Now().Unix())

	var auxIDS []string

	_inputAUX := r.URL.Query().Get("aux_ids")
	auxIDS = strings.Split(_inputAUX, ",")
	if len(_inputAUX) == 0 {
		auxIDS = []string{}
		db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte(dbName))
			return bucket.ForEach(func(name []byte, _ []byte) error {
				auxIDS = append(auxIDS, string(name))
				return nil
			})
		})
	}

	var programs []ProgramShort
	for _, aux_id := range auxIDS {

		var program ProgramEvent
		var programShort ProgramShort
		programShort.ChannelID = aux_id
		// log.Printf("%v", aux_id)
		err := db.View(func(tx *bolt.Tx) error {
			bucket := tx.Bucket([]byte(dbName)).Bucket([]byte(aux_id))

			_ = bucket
			if bucket == nil {
				return errors.New("Bucket doesnt exists, " + aux_id)
			}
			c := bucket.Cursor()
			tnow, _ := strconv.Atoi(now)

			k, v := c.Seek(itob(int(tnow)))
			if !bytes.Equal(k, []byte(now)) {
				k, v = c.Prev()
			}
			k, v = c.Prev()
			_ = v
			json.Unmarshal(v, &program)
			programShort.Events = append(programShort.Events, program)

			k, v = c.Next()
			_ = v
			json.Unmarshal(v, &program)
			programShort.Events = append(programShort.Events, program)

			k, v = c.Next()
			_ = v
			json.Unmarshal(v, &program)
			programShort.Events = append(programShort.Events, program)

			return nil
		})
		programs = append(programs, programShort)
		if err != nil {
			log.Printf("%v", err)
		}
	}
	_ = now

	resp := Response{
		Method: "short_epg",
		Status: 200,
		Response: Channels{
			Version:  time.Now().Unix(),
			Channels: programs,
		},
	}

	// w.Write(encode(programs))
	w.Write(encode(resp))

}

func fetchXML() error {
	if xmlURL == "" {
		return errors.New("xmlURL is empty")
	}
	request, _ := http.NewRequest("GET", xmlURL, nil)

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	fh, err := os.Create(xmlFile)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = io.Copy(fh, response.Body)
	if err != nil {
		return err
	}
	return nil

}
