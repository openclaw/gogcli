package cmd

import (
	"fmt"
	"time"
)

// ianaToOffset maps common IANA timezone names to their fixed UTC offsets.
// This is a fallback for Windows systems where time.LoadLocation() may not recognize
// IANA timezone names. The offset format is "UTC+/-HH:MM" which Go can parse.
var ianaToOffset = map[string]string{
	// Pacific
	"Pacific/Kiritimati":   "+13:00",
	"Pacific/Auckland":    "+13:00",
	"Pacific/Fiji":        "+13:00",
	"Pacific/Tongatapu":    "+13:00",
	"Pacific/Chatham":     "+13:45",
	"Pacific/Apia":        "+13:00",
	"Pacific/Norfolk":     "+11:30",
	"Pacific/Lord_Howe":   "+10:30", // Half timezone, seasonal
	"Pacific/Sydney":      "+10:00",
	"Pacific/Melbourne":   "+10:00",
	"Pacific/Brisbane":    "+10:00",
	"Pacific/Adelaide":    "+09:30",
	"Pacific/Darwin":      "+09:30",
	"Pacific/Port_Moresby": "+10:00",
	"Pacific/Guam":        "+10:00",
	"Pacific/Honolulu":    "-10:00",
	"Pacific/Marquesas":   "-09:30",
	"Pacific/Gambier":     "-09:00",
	"Pacific/Easter":      "-05:00",
	"Pacific/Galapagos":   "-06:00",

	// Australia (region-specific IANA names)
	"Australia/Lord_Howe": "+10:30", // Seasonal
	"Australia/Hobart":     "+10:00",
	"Australia/Currie":     "+10:00",
	"Australia/Melbourne": "+10:00",
	"Australia/Sydney":     "+10:00",
	"Australia/Broken_Hill": "+10:00",
	"Australia/Brisbane":   "+10:00",
	"Australia/Lindeman":  "+10:00",
	"Australia/Adelaide":   "+09:30",
	"Australia/Darwin":     "+09:30",
	"Australia/Perth":      "+08:00",
	"Australia/Eucla":      "+08:45",

	// New Zealand
	"NZ":                    "+12:00",
	"NZ-CHAT":               "+12:45",

	// Asia
	"Asia/Kamchatka":       "+12:00",
	"Asia/Anadyr":          "+12:00",
	"Asia/Magadan":         "+11:00",
	"Asia/Sakhalin":        "+11:00",
	"Asia/Vladivostok":     "+10:00",
	"Asia/Ust-Nera":        "+10:00",
	"Asia/Khandyga":        "+09:00",
	"Asia/Irkutsk":         "+08:00",
	"Asia/Chita":           "+08:00",
	"Asia/Yakutsk":         "+09:00",
	"Asia/Krasnoyarsk":     "+07:00",
	"Asia/Novokuznetsk":    "+07:00",
	"Asia/Novosibirsk":     "+07:00",
	"Asia/Omsk":            "+06:00",
	"Asia/Tomsk":           "+07:00",
	"Asia/Barnaul":         "+07:00",
	"Asia/Atyrau":          "+05:00",
	"Asia/Aqtobe":          "+05:00",
	"Asia/Orenburg":        "+05:00",
	"Asia/Yekaterinburg":   "+05:00",
	"Asia/Tyumen":          "+05:00",
	"Asia/Oral":            "+05:00",
	"Asia/Aqtau":           "+05:00",
	"Asia/Makat":           "+05:00",
	"Asia/Qyzylorda":       "+06:00",
	"Asia/Almaty":          "+06:00",
	"Asia/Bishkek":         "+06:00",
	"Asia/Tashkent":        "+05:00",
	"Asia/Samarkand":       "+05:00",
	"Asia/Dushanbe":        "+05:00",
	"Asia/Ashgabat":        "+05:00",
	"Asia/Tbilisi":         "+04:00",
	"Asia/Yerevan":         "+04:00",
	"Asia/Baku":            "+04:00",
	"Asia/Dubai":           "+04:00",
	"Asia/Tehran":          "+03:30",
	"Asia/Muscat":          "+04:00",
	"Asia/Kabul":           "+04:30",
	"Asia/Karachi":         "+05:00",
	"Asia/Kolkata":         "+05:30",
	"Asia/Colombo":         "+05:30",
	"Asia/Kathmandu":       "+05:45",
	"Asia/Dhaka":           "+06:00",
	"Asia/Yangon":          "+06:30",
	"Asia/Bangkok":         "+07:00",
	"Asia/Jakarta":         "+07:00",
	"Asia/Shanghai":        "+08:00",
	"Asia/Hong_Kong":       "+08:00",
	"Asia/Taipei":          "+08:00",
	"Asia/Seoul":           "+09:00",
	"Asia/Tokyo":           "+09:00",
	"Asia/Pyongyang":       "+09:00",

	// Europe
	"Europe/Moscow":        "+03:00",
	"Europe/Kaliningrad":   "+02:00",
	"Europe/Kiev":          "+02:00",
	"Europe/Chisinau":      "+02:00",
	"Europe/Istanbul":      "+03:00",
	"Europe/Athens":        "+02:00",
	"Europe/Bucharest":     "+02:00",
	"Europe/Helsinki":      "+02:00",
	"Europe/Riga":          "+02:00",
	"Europe/Vilnius":       "+02:00",
	"Europe/Tallinn":       "+02:00",
	"Europe/Warsaw":        "+01:00",
	"Europe/Prague":        "+01:00",
	"Europe/Budapest":      "+01:00",
	"Europe/Bratislava":    "+01:00",
	"Europe/Ljubljana":     "+01:00",
	"Europe/San_Marino":    "+01:00",
	"Europe/Vatican":       "+01:00",
	"Europe/Rome":          "+01:00",
	"Europe/Milan":         "+01:00",
	"Europe/Berlin":        "+01:00",
	"Europe/Paris":         "+01:00",
	"Europe/Brussels":      "+01:00",
	"Europe/Amsterdam":     "+01:00",
	"Europe/Luxembourg":    "+01:00",
	"Europe/London":        "+00:00",
	"Europe/Dublin":        "+00:00",
	"Europe/Lisbon":        "+00:00",
	"Europe/Madrid":        "+01:00",
	"Europe/Belgrade":      "+01:00",
	"Europe/Sarajevo":      "+01:00",
	"Europe/Zagreb":        "+01:00",
	"Europe/Sofia":         "+02:00",

	// Americas
	"America/Halifax":      "-04:00",
	"America/New_York":     "-04:00",
	"America/Chicago":      "-05:00",
	"America/Denver":       "-06:00",
	"America/Phoenix":      "-07:00",
	"America/Los_Angeles":  "-07:00",
	"America/Anchorage":    "-08:00",
	"America/Adak":         "-10:00",
	"America/Metlakatla":   "-08:00",
	"America/Sitka":        "-08:00",
	"America/Juneau":       "-08:00",
	"America/Vancouver":    "-07:00",
	"America/Edmonton":     "-06:00",
	"America/Winnipeg":     "-05:00",
	"America/Regina":       "-06:00",
	"America/Swift_Current": "-06:00",
	"America/Toronto":      "-04:00",
	"America/Montreal":     "-04:00",
	"America/Noronha":      "-02:00",
	"America/St_Johns":     "-03:30",
	"America/Goose_Bay":    "-03:00",
	"America/Havana":       "-04:00",
	"America/Bogota":       "-05:00",
	"America/Lima":         "-05:00",
	"America/Santiago":     "-04:00",
	"Argentina/Buenos_Aires": "-03:00",
	"America/Montevideo":   "-03:00",
	"America/Sao_Paulo":    "-03:00",
	"America/Bahia":        "-03:00",
	"America/Recife":       "-03:00",
	"America/Manaus":       "-04:00",
	"America/Eirunepe":     "-03:00",
	"America/Rio_Branco":   "-02:00",
	"America/Godthab":      "-03:00",
	"America/Miquelon":     "-02:00",

	// Africa
	"Africa/Cairo":         "+02:00",
	"Africa/Johannesburg":  "+02:00",
	"Africa/Nairobi":       "+03:00",
	"Africa/Lagos":         "+01:00",
	"Africa/Algiers":       "+01:00",
	"Africa/Casablanca":    "+01:00",
	"Africa/Tunis":         "+01:00",
	"Africa/Tripoli":       "+02:00",
	"Africa/Harare":        "+02:00",
	"Africa/Maputo":        "+02:00",
	"Africa/Dar_es_Salaam": "+03:00",
	"Africa/Kampala":       "+03:00",
	"Africa/Addis_Ababa":   "+03:00",
	"Africa/Khartoum":      "+02:00",
	"Africa/Djibouti":      "+03:00",
	"Africa/Mogadishu":     "+03:00",
	"Africa/Ndjamena":      "+01:00",
	"Africa/Brazzaville":   "+01:00",
	"Africa/Douala":        "+01:00",
	"Africa/Lubumbashi":    "+02:00",
	"Africa/Kigali":        "+02:00",
	"Africa/Bujumbura":     "+02:00",
	"Africa/Maseru":        "+02:00",
	"Africa/Mbabane":       "+02:00",
	"Africa/Windhoek":      "+01:00",
	"Africa/Gaborone":      "+02:00",

	// Indian Ocean
	"Indian/Mauritius":     "+04:00",
	"Indian/Maldives":      "+05:00",
	"Indian/Kolkata":       "+05:30",
	"Indian/Colombo":       "+05:30",
	"Indian/Chagos":        "+06:00",
	"Indian/Mayotte":       "+03:00",
	"Indian/Reunion":       "+04:00",

	// Atlantic
	"Atlantic/Canary":      "+00:00",
	"Atlantic/Faroe":       "+00:00",
	"Atlantic/Azores":      "-01:00",
	"Atlantic/Cape_Verde":  "-01:00",
	"Atlantic/Bermuda":     "-03:00",
	"Atlantic/Stanley":     "-03:00",

	// Central and Misc
	"UTC":                  "+00:00",
}

// loadLocationWithFallback attempts to load a timezone location,
// with fallback to a fixed offset for Windows compatibility.
// On Windows, time.LoadLocation() may fail for IANA timezone names like "Pacific/Auckland".
// This function provides a fallback by using the fixed UTC offset instead.
func loadLocationWithFallback(tzName string) (*time.Location, error) {
	// First try the standard Go timezone loader
	loc, err := time.LoadLocation(tzName)
	if err == nil {
		return loc, nil
	}

	// Fallback: try to get the fixed offset from our map
	offsetStr, ok := ianaToOffset[tzName]
	if !ok {
		// Try to parse as a fixed offset directly (e.g., "UTC+13:00")
		if len(tzName) > 3 && (tzName[0:3] == "UTC" || tzName[0:3] == "GMT") {
			offsetStr = tzName[3:]
		} else {
			return nil, fmt.Errorf("unknown time zone %q and no offset mapping available", tzName)
		}
	}

	// Create a fixed offset location
	// Parse the offset string to get hours and minutes
	var offsetHours, offsetMinutes int
	fmt.Sscanf(offsetStr, "%d:%d", &offsetHours, &offsetMinutes)

	// Calculate total seconds
	offsetSeconds := offsetHours*3600 + offsetMinutes*60
	if offsetStr[0] == '-' {
		offsetSeconds = -offsetSeconds
	}

	return time.FixedZone(tzName, offsetSeconds), nil
}
