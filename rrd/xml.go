package rrd

import (
    "fmt"
    "encoding/xml"
)

type RRD struct {
    Version string `xml:"version"`
    Step int `xml:"step"`
    LastUpdate int64 `xml:"lastupdate"`
    DS []DS `xml:"ds"`
    RRA []RRA `xml:"rra"`
}

type DS struct {
    Name string `xml:"name"`
    Type string `xml:"type"`
    MinimalHeartbeat int `xml:"minimal_heartbeat"`
    Min float64 `xml:"min"`
    Max float64 `xml:"max"`
    LastDS int `xml:"last_ds"`
    Value float64 `xml:"value"`
    UnknownSec SpacedInt `xml:"unknown_sec"`
}

type RRA struct {
    CF string `xml:"cf"`
    PDPPerRow int `xml:"pdp_per_row"`
    Params *Params `xml:"params"`
    CDPPrep CDPPrep `xml:"cdp_prep"`
    Database Database `xml:"database"`
}

type Params struct {
    XFF float64 `xml:"xff"`
}

type CDPPrep struct {
    DS []CDPPrepDS `xml:"ds"`
}

type CDPPrepDS struct {
    PrimaryValue float64 `xml:"primary_value"`
    SecondaryValue float64 `xml:"secondary_value"`
    Value float64 `xml:"value"`
    UnknownDatapoints float64 `xml:"unknown_datapoints"`
}

type Database struct {
    Row []Row `xml:"row"`
}

type Row struct {
    V []Value `xml:"v"`
}

type Value float64
type SpacedInt int

func (i *SpacedInt) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
    var str string
    err := d.DecodeElement(&str, &start)
    if err != nil {
        return err
    }
    _, err = fmt.Sscanf(str, " %d ", i)
    return err
}
