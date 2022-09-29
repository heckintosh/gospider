package core

import (
	"crypto/tls"
	"fmt"
	"gospider/config"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
	"github.com/jaeles-project/gospider/stringset"
)

var DefaultHTTPTransport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout: 10 * time.Second,
		// Default is 15 seconds
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:    100,
	MaxConnsPerHost: 1000,
	IdleConnTimeout: 30 * time.Second,

	// ExpectContinueTimeout: 1 * time.Second,
	// ResponseHeaderTimeout: 3 * time.Second,
	// DisableCompression:    false,
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true, Renegotiation: tls.RenegotiateOnceAsClient},
}

type Crawler struct {
	C                   *colly.Collector
	LinkFinderCollector *colly.Collector
	Output              *Output

	subSet  *stringset.StringFilter
	awsSet  *stringset.StringFilter
	jsSet   *stringset.StringFilter
	urlSet  *stringset.StringFilter
	formSet *stringset.StringFilter

	site   *url.URL
	domain string
	Input  string
	Quiet  bool
	raw    bool
	Result []string
}

type SpiderOutput struct {
	Input      string `json:"input"`
	Source     string `json:"source"`
	OutputType string `json:"type"`
	Output     string `json:"output"`
	StatusCode int    `json:"status"`
}

func NewCrawler(site *url.URL, cfg *config.SpiderCfg, logObject *logrus.Logger) *Crawler {
	domain := GetDomain(site)
	if domain == "" {
		logObject.Error("Failed to parse domain")
		os.Exit(1)
	}
	logObject.Infof("Start crawling: %s", site)

	maxDepth := cfg.Depth
	concurrent := cfg.Concurrent
	delay := cfg.Delay
	raw := cfg.Raw
	subs := cfg.Subs

	c := colly.NewCollector(
		colly.Async(true),
		colly.MaxDepth(maxDepth),
		colly.IgnoreRobotsTxt(),
	)

	// Setup http client
	client := &http.Client{}

	// Set proxy
	proxy := cfg.Proxy
	if proxy != "" {
		logObject.Infof("Proxy: %s", proxy)
		pU, err := url.Parse(proxy)
		if err != nil {
			logObject.Error("Failed to set proxy")
		} else {
			DefaultHTTPTransport.Proxy = http.ProxyURL(pU)
		}
	}

	// Set request timeout
	timeout := cfg.Timeout
	if timeout == 0 {
		logObject.Info("Your input timeout is 0. Gospider will set it to 10 seconds")
		client.Timeout = 10 * time.Second
	} else {
		client.Timeout = time.Duration(timeout) * time.Second
	}

	// Disable redirect
	noRedirect := cfg.NoRedirect
	if noRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			nextLocation := req.Response.Header.Get("Location")
			logObject.Debugf("Found Redirect: %s", nextLocation)

			// Allow in redirect from http to https or in same hostname
			// We just check contain hostname or not because we set URLFilter in main collector so if
			// the URL is https://otherdomain.com/?url=maindomain.com, it will reject it
			if strings.Contains(nextLocation, site.Hostname()) {
				logObject.Infof("Redirecting to: %s", nextLocation)
				return nil
			}
			return http.ErrUseLastResponse
		}
	}

	// Set client transport
	client.Transport = DefaultHTTPTransport
	c.SetClient(client)

	// Set cookies
	cookie := cfg.Cookie
	if cookie != "" {
		c.OnRequest(func(r *colly.Request) {
			r.Headers.Set("Cookie", cookie)
		})
	}

	// Set headers
	headers := cfg.Header
	for _, h := range headers {
		headerArgs := strings.SplitN(h, ":", 2)
		headerKey := strings.TrimSpace(headerArgs[0])
		headerValue := strings.TrimSpace(headerArgs[1])
		c.OnRequest(func(r *colly.Request) {
			r.Headers.Set(headerKey, headerValue)
		})
	}

	// Set User-Agent
	randomUA := cfg.UserAgent
	switch ua := strings.ToLower(randomUA); {
	case ua == "mobi":
		extensions.RandomMobileUserAgent(c)
	case ua == "web":
		extensions.RandomUserAgent(c)
	default:
		c.UserAgent = ua
	}

	// Set referer
	extensions.Referer(c)

	// Init Output
	var output *Output

	// Set url whitelist regex
	reg := ""
	if subs {
		reg = site.Hostname()
	} else {
		reg = "(?:https|http)://" + site.Hostname()
	}

	sRegex := regexp.MustCompile(reg)
	c.URLFilters = append(c.URLFilters, sRegex)

	// Set Limit Rule
	err := c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: concurrent,
		Delay:       time.Duration(delay) * time.Second,
	})
	if err != nil {
		logObject.Errorf("Failed to set Limit Rule: %s", err)
		os.Exit(1)
	}

	// GoSpider default disallowed regex
	disallowedRegex := `(?i)\.(png|apng|bmp|gif|ico|cur|jpg|jpeg|jfif|pjp|pjpeg|svg|tif|tiff|webp|xbm|3gp|aac|flac|mpg|mpeg|mp3|mp4|m4a|m4v|m4p|oga|ogg|ogv|mov|wav|webm|eot|woff|woff2|ttf|otf|css)(?:\?|#|$)`
	c.DisallowedURLFilters = append(c.DisallowedURLFilters, regexp.MustCompile(disallowedRegex))

	// Set optional blacklist url regex
	blacklists := cfg.Blacklist
	if blacklists != "" {
		c.DisallowedURLFilters = append(c.DisallowedURLFilters, regexp.MustCompile(blacklists))
	}

	// Set optional whitelist url regex
	whiteLists := cfg.Whitelist
	if whiteLists != "" {
		c.URLFilters = make([]*regexp.Regexp, 0)
		c.URLFilters = append(c.URLFilters, regexp.MustCompile(whiteLists))
	}

	whiteListDomain := cfg.WhitelistDomain
	if whiteListDomain != "" {
		c.URLFilters = make([]*regexp.Regexp, 0)
		c.URLFilters = append(c.URLFilters, regexp.MustCompile("http(s)?://"+whiteListDomain))
	}

	linkFinderCollector := c.Clone()
	// Try to request as much as Javascript source and don't care about domain.
	// The result of link finder will be send to Link Finder Collector to check is it working or not.
	linkFinderCollector.URLFilters = nil
	if whiteLists != "" {
		linkFinderCollector.URLFilters = append(linkFinderCollector.URLFilters, regexp.MustCompile(whiteLists))
	}
	if whiteListDomain != "" {
		linkFinderCollector.URLFilters = append(linkFinderCollector.URLFilters, regexp.MustCompile("http(s)?://"+whiteListDomain))
	}
	var result []string

	return &Crawler{
		C:                   c,
		LinkFinderCollector: linkFinderCollector,
		site:                site,
		Quiet:               true,
		Input:               site.String(),
		raw:                 raw,
		domain:              domain,
		Output:              output,
		urlSet:              stringset.NewStringFilter(),
		subSet:              stringset.NewStringFilter(),
		jsSet:               stringset.NewStringFilter(),
		formSet:             stringset.NewStringFilter(),
		awsSet:              stringset.NewStringFilter(),
		Result:              result,
	}
}

func (crawler *Crawler) feedLinkfinder(jsFileUrl string, OutputType string, source string) {
	if !crawler.jsSet.Duplicate(jsFileUrl) {
		// If JS file is minimal format. Try to find original format
		if strings.Contains(jsFileUrl, ".min.js") {
			originalJS := strings.ReplaceAll(jsFileUrl, ".min.js", ".js")
			_ = crawler.LinkFinderCollector.Visit(originalJS)
		}

		// Send Javascript to Link Finder Collector
		_ = crawler.LinkFinderCollector.Visit(jsFileUrl)

	}
}

func (crawler *Crawler) Start(linkfinder bool, logObject *logrus.Logger) {
	// Setup Link Finder
	if linkfinder {
		crawler.setupLinkFinder(logObject)
	}

	// Handle url
	crawler.C.OnHTML("[href]", func(e *colly.HTMLElement) {
		urlString := e.Request.AbsoluteURL(e.Attr("href"))
		urlString = FixUrl(crawler.site, urlString)
		if urlString == "" {
			return
		}
		if !crawler.urlSet.Duplicate(urlString) {
			_ = e.Request.Visit(urlString)
		}
	})

	// Handle form
	crawler.C.OnHTML("form[action]", func(e *colly.HTMLElement) {
		_ = e.Request.URL
	})

	// Find Upload Form
	crawler.C.OnHTML(`input[type="file"]`, func(e *colly.HTMLElement) {
		_ = e.Request.URL.String()
	})

	// Handle js files
	crawler.C.OnHTML("[src]", func(e *colly.HTMLElement) {
		jsFileUrl := e.Request.AbsoluteURL(e.Attr("src"))
		jsFileUrl = FixUrl(crawler.site, jsFileUrl)
		if jsFileUrl == "" {
			return
		}

		fileExt := GetExtType(jsFileUrl)
		if fileExt == ".js" || fileExt == ".xml" || fileExt == ".json" {
			crawler.feedLinkfinder(jsFileUrl, "javascript", "body")
		}
	})
	crawler.C.OnResponse(func(response *colly.Response) {
		respStr := DecodeChars(string(response.Body))

		// Verify which link is working
		u := response.Request.URL.String()
		fmt.Println(u)
		if InScope(response.Request.URL, crawler.C.URLFilters) {
			crawler.findAWSS3(respStr)
		}
	})

	err := crawler.C.Visit(crawler.site.String())
	if err != nil {
		logObject.Errorf("Failed to start %s: %s", crawler.site.String(), err)
	}
}

// Find AWS S3 from response
func (crawler *Crawler) findAWSS3(resp string) {
	aws := GetAWSS3(resp)
	for _, e := range aws {
		if !crawler.awsSet.Duplicate(e) {
			outputFormat := fmt.Sprintf("[aws-s3] - %s", e)
			if crawler.Output != nil {
				crawler.Output.WriteToFile(outputFormat)
			}
		}
	}
}

// Setup link finder
func (crawler *Crawler) setupLinkFinder(logObject *logrus.Logger) {
	crawler.LinkFinderCollector.OnResponse(func(response *colly.Response) {
		if response.StatusCode == 404 || response.StatusCode == 429 || response.StatusCode < 100 {
			return
		}

		respStr := string(response.Body)

		// Verify which link is working
		u := response.Request.URL.String()

		if InScope(response.Request.URL, crawler.C.URLFilters) {

			crawler.findAWSS3(respStr)

			paths, err := LinkFinder(respStr)
			if err != nil {
				logObject.Error(err)
				return
			}

			currentPathURL, err := url.Parse(u)
			currentPathURLerr := false
			if err != nil {
				currentPathURLerr = true
			}

			for _, relPath := range paths {

				rebuildURL := ""
				if !currentPathURLerr {
					rebuildURL = FixUrl(currentPathURL, relPath)
				} else {
					rebuildURL = FixUrl(crawler.site, relPath)
				}

				if rebuildURL == "" {
					continue
				}

				// Try to request JS path
				// Try to generate URLs with main site
				fileExt := GetExtType(rebuildURL)
				if fileExt == ".js" || fileExt == ".xml" || fileExt == ".json" || fileExt == ".map" {
					crawler.feedLinkfinder(rebuildURL, "linkfinder", "javascript")
				} else if !crawler.urlSet.Duplicate(rebuildURL) {
					_ = crawler.C.Visit(rebuildURL)
				}

				// Try to generate URLs with the site where Javascript file host in (must be in main or sub domain)

				urlWithJSHostIn := FixUrl(crawler.site, relPath)
				if urlWithJSHostIn != "" {
					fileExt := GetExtType(urlWithJSHostIn)
					if fileExt == ".js" || fileExt == ".xml" || fileExt == ".json" || fileExt == ".map" {
						crawler.feedLinkfinder(urlWithJSHostIn, "linkfinder", "javascript")
					} else {
						if crawler.urlSet.Duplicate(urlWithJSHostIn) {
							continue
						} else {
							_ = crawler.C.Visit(urlWithJSHostIn) //not print care for lost link
						}
					}

				}

			}
		}
	})
}
