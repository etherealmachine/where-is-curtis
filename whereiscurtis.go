package whereiscurtis

import (
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

const (
	LocationKind = "Location"

	feedLocation = "https://api.findmespot.com/spot-main-web/consumer/rest-api/2.0/public/feed/0Yqto1SBp1MrHmtmldF3mTYWBRej4cKNC/message.xml"
)

type Location struct {
	UnixTime  int64   `xml:"unixTime"`
	Latitude  float64 `xml:"latitude"`
	Longitude float64 `xml:"longitude"`
}

func loadLocations(ctx context.Context) ([]*Location, error) {
	var locs []*Location
	q := datastore.NewQuery(LocationKind).Order("UnixTime")
	for t := q.Run(ctx); ; {
		var loc Location
		if _, err := t.Next(&loc); err == datastore.Done {
			break
		} else if err != nil {
			return nil, err
		}
		locs = append(locs, &loc)
	}
	return locs, nil
}

func handleLocationRequest(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	locs, err := loadLocations(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	e := json.NewEncoder(w)
	e.Encode(locs)
}

func handleIngestRequest(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	c := urlfetch.Client(ctx)
	resp, err := c.Get(feedLocation)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		handleError(ctx, w, err)
		return
	}
	xmlResp := &struct {
		Locations []*Location `xml:"feedMessageResponse>messages>message"`
	}{}
	if err := xml.Unmarshal(bs, xmlResp); err != nil {
		handleError(ctx, w, err)
		return
	}

	newEntries := 0
	for _, loc := range xmlResp.Locations {
		key := datastore.NewKey(ctx, LocationKind, "", loc.UnixTime, nil)
		var exists Location
		if err := datastore.Get(ctx, key, &exists); err == datastore.ErrNoSuchEntity {
			newEntries++
			if _, err := datastore.Put(ctx, key, loc); err != nil {
				handleError(ctx, w, err)
				return
			}
		} else if err != nil {
			handleError(ctx, w, err)
			return
		}
	}

	log.Infof(ctx, "%d new location entries", newEntries)
	e := json.NewEncoder(w)
	e.Encode(xmlResp.Locations)
}

func handleError(ctx context.Context, w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte(err.Error()))
	log.Errorf(ctx, "%v", err)
}

func init() {
	http.Handle("/tasks/ingest", http.HandlerFunc(handleIngestRequest))
	http.Handle("/locations.json", http.HandlerFunc(handleLocationRequest))
}
