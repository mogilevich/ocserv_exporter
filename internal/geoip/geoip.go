package geoip

import (
	"log"
	"net"

	"github.com/oschwald/geoip2-golang"
)

// Resolver provides GeoIP lookups using MaxMind GeoLite2 database
type Resolver struct {
	db *geoip2.Reader
}

// NewResolver creates a new GeoIP resolver
// dbPath should point to a GeoLite2-Country.mmdb file
func NewResolver(dbPath string) (*Resolver, error) {
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &Resolver{db: db}, nil
}

// Lookup returns country name and ISO code for an IP address
func (r *Resolver) Lookup(ipStr string) (country, countryCode string) {
	if r.db == nil {
		return "", ""
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", ""
	}

	// Skip private/internal IPs
	if ip.IsPrivate() || ip.IsLoopback() {
		return "Private", "XX"
	}

	record, err := r.db.Country(ip)
	if err != nil {
		log.Printf("GeoIP lookup error for %s: %v", ipStr, err)
		return "", ""
	}

	country = record.Country.Names["en"]
	countryCode = record.Country.IsoCode

	if country == "" {
		country = "Unknown"
		countryCode = "ZZ"
	}

	return country, countryCode
}

// Close closes the GeoIP database
func (r *Resolver) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}
