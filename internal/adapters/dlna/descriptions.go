package dlna

import (
	"fmt"
	"strings"
)

// Service and device types this server implements.
const (
	deviceType        = "urn:schemas-upnp-org:device:MediaServer:1"
	contentDirectory  = "urn:schemas-upnp-org:service:ContentDirectory:1"
	connectionManager = "urn:schemas-upnp-org:service:ConnectionManager:1"
)

// URL paths, all under /dlna/. Clients get them from the device description, so
// they are only ever followed relative to the address the description came from.
const (
	basePath         = "/dlna/"
	devicePath       = basePath + "device.xml"
	cdSCPDPath       = basePath + "cd.xml"
	cdControlPath    = basePath + "cd/control"
	cdEventPath      = basePath + "cd/event"
	cmSCPDPath       = basePath + "cm.xml"
	cmControlPath    = basePath + "cm/control"
	cmEventPath      = basePath + "cm/event"
	deviceXMLContent = `text/xml; charset="utf-8"`
)

// deviceXML describes the MediaServer: who it is and where its two services are.
func deviceXML(name, uuid, version string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0" xmlns:dlna="urn:schemas-dlna-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<device>
<dlna:X_DLNADOC>DMS-1.50</dlna:X_DLNADOC>
<deviceType>%s</deviceType>
<friendlyName>%s</friendlyName>
<manufacturer>EinCast</manufacturer>
<modelName>EinCast</modelName>
<modelDescription>Your home media, on every screen.</modelDescription>
<modelNumber>%s</modelNumber>
<UDN>uuid:%s</UDN>
<serviceList>
<service><serviceType>%s</serviceType><serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId><SCPDURL>%s</SCPDURL><controlURL>%s</controlURL><eventSubURL>%s</eventSubURL></service>
<service><serviceType>%s</serviceType><serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId><SCPDURL>%s</SCPDURL><controlURL>%s</controlURL><eventSubURL>%s</eventSubURL></service>
</serviceList>
</device>
</root>`,
		deviceType, text(name), text(version), text(uuid),
		contentDirectory, cdSCPDPath, cdControlPath, cdEventPath,
		connectionManager, cmSCPDPath, cmControlPath, cmEventPath)
}

// arg describes one argument of an action in a service description.
type arg struct{ name, dir, related string }

// action is one action of a service description.
type action struct {
	name string
	args []arg
}

// stateVar is one state variable of a service description.
type stateVar struct {
	name, typ string
	events    bool
	allowed   []string
}

// scpd writes a service description: the actions a client may call and the
// types of their arguments.
func scpd(actions []action, vars []stateVar) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<scpd xmlns="urn:schemas-upnp-org:service-1-0"><specVersion><major>1</major><minor>0</minor></specVersion><actionList>`)
	for _, a := range actions {
		fmt.Fprintf(&b, `<action><name>%s</name><argumentList>`, a.name)
		for _, g := range a.args {
			fmt.Fprintf(&b, `<argument><name>%s</name><direction>%s</direction><relatedStateVariable>%s</relatedStateVariable></argument>`, g.name, g.dir, g.related)
		}
		b.WriteString(`</argumentList></action>`)
	}
	b.WriteString(`</actionList><serviceStateTable>`)
	for _, v := range vars {
		ev := "no"
		if v.events {
			ev = "yes"
		}
		fmt.Fprintf(&b, `<stateVariable sendEvents="%s"><name>%s</name><dataType>%s</dataType>`, ev, v.name, v.typ)
		if len(v.allowed) > 0 {
			b.WriteString(`<allowedValueList>`)
			for _, a := range v.allowed {
				fmt.Fprintf(&b, `<allowedValue>%s</allowedValue>`, a)
			}
			b.WriteString(`</allowedValueList>`)
		}
		b.WriteString(`</stateVariable>`)
	}
	b.WriteString(`</serviceStateTable></scpd>`)
	return b.String()
}

// contentDirectorySCPD describes ContentDirectory:1 with the actions that
// browsing needs. Search is deliberately absent: GetSearchCapabilities says so.
func contentDirectorySCPD() string {
	return scpd([]action{
		{"GetSearchCapabilities", []arg{{"SearchCaps", "out", "SearchCapabilities"}}},
		{"GetSortCapabilities", []arg{{"SortCaps", "out", "SortCapabilities"}}},
		{"GetSystemUpdateID", []arg{{"Id", "out", "SystemUpdateID"}}},
		{"Browse", []arg{
			{"ObjectID", "in", "A_ARG_TYPE_ObjectID"},
			{"BrowseFlag", "in", "A_ARG_TYPE_BrowseFlag"},
			{"Filter", "in", "A_ARG_TYPE_Filter"},
			{"StartingIndex", "in", "A_ARG_TYPE_Index"},
			{"RequestedCount", "in", "A_ARG_TYPE_Count"},
			{"SortCriteria", "in", "A_ARG_TYPE_SortCriteria"},
			{"Result", "out", "A_ARG_TYPE_Result"},
			{"NumberReturned", "out", "A_ARG_TYPE_Count"},
			{"TotalMatches", "out", "A_ARG_TYPE_Count"},
			{"UpdateID", "out", "A_ARG_TYPE_UpdateID"},
		}},
	}, []stateVar{
		{"A_ARG_TYPE_ObjectID", "string", false, nil},
		{"A_ARG_TYPE_Result", "string", false, nil},
		{"A_ARG_TYPE_BrowseFlag", "string", false, []string{"BrowseMetadata", "BrowseDirectChildren"}},
		{"A_ARG_TYPE_Filter", "string", false, nil},
		{"A_ARG_TYPE_SortCriteria", "string", false, nil},
		{"A_ARG_TYPE_Index", "ui4", false, nil},
		{"A_ARG_TYPE_Count", "ui4", false, nil},
		{"A_ARG_TYPE_UpdateID", "ui4", false, nil},
		{"SearchCapabilities", "string", false, nil},
		{"SortCapabilities", "string", false, nil},
		{"SystemUpdateID", "ui4", true, nil},
	})
}

// connectionManagerSCPD describes ConnectionManager:1, which a MediaServer must
// have but which, for a server that only hands out URLs, has little to say.
func connectionManagerSCPD() string {
	return scpd([]action{
		{"GetProtocolInfo", []arg{{"Source", "out", "SourceProtocolInfo"}, {"Sink", "out", "SinkProtocolInfo"}}},
		{"GetCurrentConnectionIDs", []arg{{"ConnectionIDs", "out", "CurrentConnectionIDs"}}},
		{"GetCurrentConnectionInfo", []arg{
			{"ConnectionID", "in", "A_ARG_TYPE_ConnectionID"},
			{"RcsID", "out", "A_ARG_TYPE_RcsID"},
			{"AVTransportID", "out", "A_ARG_TYPE_AVTransportID"},
			{"ProtocolInfo", "out", "A_ARG_TYPE_ProtocolInfo"},
			{"PeerConnectionManager", "out", "A_ARG_TYPE_ConnectionManager"},
			{"PeerConnectionID", "out", "A_ARG_TYPE_ConnectionID"},
			{"Direction", "out", "A_ARG_TYPE_Direction"},
			{"Status", "out", "A_ARG_TYPE_ConnectionStatus"},
		}},
	}, []stateVar{
		{"SourceProtocolInfo", "string", true, nil},
		{"SinkProtocolInfo", "string", true, nil},
		{"CurrentConnectionIDs", "string", true, nil},
		{"A_ARG_TYPE_ConnectionStatus", "string", false, []string{"OK", "ContentFormatMismatch", "InsufficientBandwidth", "UnreliableChannel", "Unknown"}},
		{"A_ARG_TYPE_ConnectionManager", "string", false, nil},
		{"A_ARG_TYPE_Direction", "string", false, []string{"Input", "Output"}},
		{"A_ARG_TYPE_ProtocolInfo", "string", false, nil},
		{"A_ARG_TYPE_ConnectionID", "i4", false, nil},
		{"A_ARG_TYPE_AVTransportID", "i4", false, nil},
		{"A_ARG_TYPE_RcsID", "i4", false, nil},
	})
}
