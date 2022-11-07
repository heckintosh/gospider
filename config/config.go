package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

type SpiderCfg struct {
	Site               string   `yaml:"site"`
	Sites              string   `yaml:"sites"`
	Proxy              string   `yaml:"proxy"`
	Output             string   `yaml:"output"`
	UserAgent          string   `yaml:"user-agent"`
	Cookie             string   `yaml:"cookie"`
	Header             []string `yaml:"header"`
	Burp               string   `yaml:"burp"`
	Blacklist          string   `yaml:"blacklist"`
	Whitelist          string   `yaml:"whitelist"`
	WhitelistDomain    string   `yaml:"whitelist-domain"`
	FilterLength       string   `yaml:"filter-length"`
	Threads            int      `yaml:"threads"`
	Concurrent         int      `yaml:"concurrent"`
	Depth              int      `yaml:"depth"`
	Delay              int      `yaml:"delay"`
	RandomDelay        int      `yaml:"random-delay"`
	Timeout            int      `yaml:"timeout"`
	Base               bool     `yaml:"B"`
	Sitemap            bool     `yaml:"sitemap"`
	Robots             bool     `yaml:"robots"`
	OtherSource        bool     `yaml:"other-source"`
	IncludeSubs        bool     `yaml:"include-subs"`
	IncludeOtherSource bool     `yaml:"include-other-source"`
	Subs               bool     `yaml:"subs"`
	Js                 bool     `yaml:"js"`
	Debug              bool     `yaml:"true"`
	Json               bool     `yaml:"json"`
	Verbose            bool     `yaml:"verbose"`
	Quiet              bool     `yaml:"quiet"`
	NoRedirect         bool     `yaml:"no-redirect"`
	Version            bool     `yaml:"version"`
	Length             bool     `yaml:"length"`
	Raw                bool     `yaml:"raw"`
	TrackerLength      int      `yaml:"tracker-length"`
	BlacklistAfter     int      `yaml:"blacklist-after"`
}

func LoadSpiderCfg(yaml_file string) (*SpiderCfg, error) {
	// default config
	c := SpiderCfg{}

	// load from YAML config file
	bytes, err := ioutil.ReadFile(yaml_file)
	if err != nil {
		return nil, err
	}
	if err = yaml.Unmarshal(bytes, &c); err != nil {
		return nil, err
	}

	return &c, err
}
