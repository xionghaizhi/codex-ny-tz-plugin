package main

import (
	"regexp"
	"sync/atomic"
	"time"
)

const defaultTZ = "America/New_York"

var tzName atomic.Value

func init() { tzName.Store(defaultTZ) }

func rewriteEnvContext(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	tz, _ := tzName.Load().(string)
	if tz == "" {
		tz = defaultTZ
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation(defaultTZ)
		tz = defaultTZ
	}
	date := time.Now().In(loc).Format("2006-01-02")
	s := string(body)
	s = replaceTag(s, "timezone", tz)
	s = replaceTag(s, "current_date", date)
	return []byte(s)
}

var (
	reTZRaw   = regexp.MustCompile(`(?s)<timezone>[^<]*</timezone>`)
	reTZEsc   = regexp.MustCompile(`(?s)\\u003ctimezone\\u003e.*?\\u003c/timezone\\u003e`)
	reDateRaw = regexp.MustCompile(`(?s)<current_date>[^<]*</current_date>`)
	reDateEsc = regexp.MustCompile(`(?s)\\u003ccurrent_date\\u003e.*?\\u003c/current_date\\u003e`)
)

func replaceTag(s, tag, val string) string {
	if tag == "timezone" {
		s = reTZRaw.ReplaceAllString(s, "<timezone>"+val+"</timezone>")
		s = reTZEsc.ReplaceAllString(s, "\\u003ctimezone\\u003e"+val+"\\u003c/timezone\\u003e")
	} else if tag == "current_date" {
		s = reDateRaw.ReplaceAllString(s, "<current_date>"+val+"</current_date>")
		s = reDateEsc.ReplaceAllString(s, "\\u003ccurrent_date\\u003e"+val+"\\u003c/current_date\\u003e")
	}
	return s
}
