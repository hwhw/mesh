package rrd

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"strconv"
	"time"
)

type Value float64
type SpacedInt int
type SpacedString string
type UnixTimestamp time.Time
type DurationSeconds time.Duration

type RRD struct {
	Version    string          `xml:"version"`
	Step       DurationSeconds `xml:"step"`
	LastUpdate UnixTimestamp   `xml:"lastupdate"`
	DS         []DS            `xml:"ds"`
	RRA        []RRA           `xml:"rra"`
}

type DS struct {
	Name             SpacedString `xml:"name"`
	Type             SpacedString `xml:"type"`
	MinimalHeartbeat int          `xml:"minimal_heartbeat"`
	Min              float64      `xml:"min"`
	Max              float64      `xml:"max"`
	LastDS           int          `xml:"last_ds"`
	Value            float64      `xml:"value"`
	UnknownSec       SpacedInt    `xml:"unknown_sec"`
}

type RRA struct {
	CF        string   `xml:"cf"`
	PDPPerRow int      `xml:"pdp_per_row"`
	Params    *Params  `xml:"params"`
	CDPPrep   CDPPrep  `xml:"cdp_prep"`
	Database  Database `xml:"database"`
}

type Params struct {
	XFF float64 `xml:"xff"`
}

type CDPPrep struct {
	DS []CDPPrepDS `xml:"ds"`
}

type CDPPrepDS struct {
	PrimaryValue      float64 `xml:"primary_value"`
	SecondaryValue    float64 `xml:"secondary_value"`
	Value             float64 `xml:"value"`
	UnknownDatapoints float64 `xml:"unknown_datapoints"`
}

type Database struct {
	Row []Row `xml:"row"`
}

type Row struct {
	V []Value `xml:"v"`
}

func (i *SpacedInt) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var str string
	err := d.DecodeElement(&str, &start)
	if err != nil {
		return err
	}
	parsed, err := strconv.Atoi(strings.Trim(str, " \n\t"))
	*i = SpacedInt(parsed)
	return err
}

func (t *UnixTimestamp) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var ts int64
	err := d.DecodeElement(&ts, &start)
	if err != nil {
		return err
	}
	*t = UnixTimestamp(time.Unix(ts, 0))
	return nil
}

func (s *SpacedString) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var str string
	err := d.DecodeElement(&str, &start)
	if err != nil {
		return err
	}
	*s = SpacedString(strings.Trim(str, " \n\t"))
	return nil
}

func (dur *DurationSeconds) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var ds int64
	err := d.DecodeElement(&ds, &start)
	if err != nil {
		return err
	}
	*dur = DurationSeconds(time.Second * time.Duration(ds))
	return nil
}

func Parse(r io.Reader) (*RRD, error) {
	dec := xml.NewDecoder(r)
	var rrd RRD
	err := dec.Decode(&rrd)
	return &rrd, err
}

type Logitem struct {
	Timestamp time.Time
	Value     Value
}

func (r *RRD) ToLogdata(out chan<- map[string]Logitem) error {
    names := make([]string, 0, 5)
	for _, ds := range r.DS {
		if ds.Type != "GAUGE" {
			return errors.New("unknown dataset type")
		}
        names = append(names, string(ds.Name))
    }

    for _, rra := range r.RRA {
        step := time.Duration(rra.PDPPerRow) * time.Duration(r.Step)
        last_item := time.Time(r.LastUpdate).Round(step)
        if time.Time(r.LastUpdate).Before(last_item) {
            // correction for round-up behaviour
            last_item = last_item.Add(-step)
        }
        first_item := last_item.Add(-step * time.Duration(len(rra.Database.Row)-1))
        for row, items := range rra.Database.Row {
            data := make(map[string]Logitem)
            for nr, v := range items.V {
                data[names[nr]] = Logitem{Timestamp: first_item.Add(step * time.Duration(row)), Value: v}
            }
            out <- data
        }
    }
	return nil
}
