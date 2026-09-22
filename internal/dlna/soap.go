package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/observability"
)

const (
	soapContentType = `text/xml; charset="utf-8"`
	cdServiceType   = "urn:schemas-upnp-org:service:ContentDirectory:1"
	cmServiceType   = "urn:schemas-upnp-org:service:ConnectionManager:1"
	soapBodyMax     = 1 << 20
)

// SOAPHandler handles ContentDirectory + ConnectionManager actions.
type SOAPHandler struct {
	Browser *Browser
	TVUser  func() (auth.PublicUser, error)
}

// ServeContentDirectory handles ContentDirectory control SOAP.
func (h *SOAPHandler) ServeContentDirectory(writer http.ResponseWriter, request *http.Request) {
	h.serveSOAP(writer, request, true)
}

// ServeConnectionManager handles ConnectionManager control SOAP.
func (h *SOAPHandler) ServeConnectionManager(writer http.ResponseWriter, request *http.Request) {
	h.serveSOAP(writer, request, false)
}

func (h *SOAPHandler) serveSOAP(
	writer http.ResponseWriter,
	request *http.Request,
	contentDirectory bool,
) {
	if request.Method != http.MethodPost {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, soapBodyMax))
	if err != nil {
		http.Error(writer, "bad request", http.StatusBadRequest)

		return
	}

	action := soapActionName(request.Header.Get("SoapAction"))
	user, userErr := h.TVUser()
	if userErr != nil {
		observability.RecordDLNASOAPRequest(action, "unauthorized")
		writeSOAPFault(writer, "401", userErr.Error())

		return
	}

	var result string
	if contentDirectory {
		result, err = h.handleContentDirectory(request, user, action, body)
	} else {
		result, err = h.handleConnectionManager(action)
	}
	if err != nil {
		observability.RecordDLNASOAPRequest(action, "error")
		writeSOAPFault(writer, "701", err.Error())

		return
	}

	observability.RecordDLNASOAPRequest(action, "success")
	writer.Header().Set("Content-Type", soapContentType)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(result))
}

func (h *SOAPHandler) handleContentDirectory(
	request *http.Request,
	user auth.PublicUser,
	action string,
	body []byte,
) (string, error) {
	switch action {
	case "Browse":
		objectID := soapTag(body, "ObjectID")
		if objectID == "" {
			objectID = "0"
		}
		flag := soapTag(body, "BrowseFlag")
		if flag != "" && flag != "BrowseDirectChildren" {
			return "", ErrUnsupportedBrowseFlag
		}
		start, _ := strconv.Atoi(soapTag(body, "StartingIndex"))
		count, _ := strconv.Atoi(soapTag(body, "RequestedCount"))
		didl, total, err := h.Browser.BrowseDirectChildren(
			request.Context(), user, objectID, start, count,
		)
		if err != nil {
			return "", fmt.Errorf("browse children: %w", err)
		}

		return soapEnvelope(
			cdServiceType,
			"Browse",
			fmt.Sprintf(
				"<Result>%s</Result><NumberReturned>%d</NumberReturned>"+
					"<TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>",
				EscapeXML(didl),
				xmlChildCount(didl),
				total,
			),
		), nil
	case "GetSearchCapabilities":
		return soapEnvelope(cdServiceType, "GetSearchCapabilities", "<SearchCaps></SearchCaps>"), nil
	case "GetSortCapabilities":
		return soapEnvelope(cdServiceType, "GetSortCapabilities", "<SortCaps></SortCaps>"), nil
	case "GetSystemUpdateID":
		return soapEnvelope(cdServiceType, "GetSystemUpdateID", "<Id>1</Id>"), nil
	default:
		return "", ErrUnknownSOAPAction
	}
}

func (h *SOAPHandler) handleConnectionManager(action string) (string, error) {
	switch action {
	case "GetProtocolInfo":
		return soapEnvelope(
			cmServiceType,
			"GetProtocolInfo",
			"<Source>http-get:*:video/mp4:*,http-get:*:video/mpeg:*</Source><Sink></Sink>",
		), nil
	case "GetCurrentConnectionIDs":
		return soapEnvelope(cmServiceType, "GetCurrentConnectionIDs", "<ConnectionIDs>0</ConnectionIDs>"), nil
	case "GetCurrentConnectionInfo":
		return soapEnvelope(
			cmServiceType,
			"GetCurrentConnectionInfo",
			"<RcsID>-1</RcsID><AVTransportID>-1</AVTransportID>"+
				"<ProtocolInfo></ProtocolInfo><PeerConnectionManager></PeerConnectionManager>"+
				"<PeerConnectionID>-1</PeerConnectionID><Direction>Output</Direction><Status>OK</Status>",
		), nil
	default:
		return "", ErrUnknownSOAPAction
	}
}

func soapActionName(headers ...string) string {
	for _, header := range headers {
		header = strings.TrimSpace(header)
		header = strings.Trim(header, "\"")
		if idx := strings.LastIndex(header, "#"); idx >= 0 {
			return header[idx+1:]
		}
	}

	return ""
}

func soapTag(body []byte, name string) string {
	start := bytes.Index(body, []byte("<"+name))
	if start < 0 {
		needle := []byte(":" + name)
		idx := bytes.Index(body, needle)
		if idx < 0 {
			return ""
		}
		start = idx
		for start > 0 && body[start] != '<' {
			start--
		}
	}
	gt := bytes.IndexByte(body[start:], '>')
	if gt < 0 {
		return ""
	}
	contentStart := start + gt + 1
	end := bytes.Index(body[contentStart:], []byte("</"))
	if end < 0 {
		return ""
	}
	value := string(body[contentStart : contentStart+end])
	if amp := strings.Index(value, "<"); amp >= 0 {
		value = value[:amp]
	}

	return xmlUnescape(strings.TrimSpace(value))
}

func xmlUnescape(value string) string {
	value = strings.ReplaceAll(value, "&lt;", "<")
	value = strings.ReplaceAll(value, "&gt;", ">")
	value = strings.ReplaceAll(value, "&quot;", "\"")
	value = strings.ReplaceAll(value, "&apos;", "'")
	value = strings.ReplaceAll(value, "&amp;", "&")

	return value
}

func soapEnvelope(serviceType, action, inner string) string {
	return xml.Header +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><u:` + action + `Response xmlns:u="` + serviceType + `">` +
		inner +
		`</u:` + action + `Response></s:Body></s:Envelope>`
}

func writeSOAPFault(writer http.ResponseWriter, code, message string) {
	body := xml.Header +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" ` +
		`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<s:Fault><faultcode>s:Client</faultcode><faultstring>` + EscapeXML(message) +
		`</faultstring><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">` +
		`<errorCode>` + code + `</errorCode><errorDescription>` + EscapeXML(message) +
		`</errorDescription></UPnPError></detail></s:Fault></s:Body></s:Envelope>`
	writer.Header().Set("Content-Type", soapContentType)
	writer.WriteHeader(http.StatusInternalServerError)
	_, _ = writer.Write([]byte(body))
}

func xmlChildCount(didl string) int {
	return strings.Count(didl, "<container") + strings.Count(didl, "<item")
}
