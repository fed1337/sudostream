package anidb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sudoStream/internal/provider"
	"time"
)

const (
	udpProtoVer     = "3"
	udpClientVer    = "1"
	udpReadTimeout  = 15 * time.Second
	udpMaxPacket    = 1400
	udpMaxRetries   = 1
	fileFMaskFull   = "60E0000000"
	fileAMaskFull   = "80E0F000"
	fileFMaskSimple = "6000000000"
	fileAMaskSimple = "00000000"
	udpReplySplitN  = 2

	// AniDB UDP reply codes (wiki.anidb.net/UDP_API).
	udpCodeLoginOK        = 200
	udpCodeLoginNewVer    = 201
	udpCodeFile           = 220
	udpCodeNoSuchFile     = 320
	udpCodeLoginFailed    = 500
	udpCodeLoginFirst     = 501
	udpCodeClientOutdated = 503
	udpCodeClientBanned   = 504
	udpCodeIllegalInput   = 505
	udpCodeInvalidSession = 506
	udpCodeBanned         = 555
	udpCodeOutOfService   = 601
)

// Assumed FILE pipe fields for fmask=60E0000000 amask=80E0F000 (after mandatory fid):
//
//	fmask byte1 0x60 → aid, eid
//	fmask byte2 0xE0 → size, ed2k, md5
//	amask byte1 0x80 → anime total episodes
//	amask byte2 0xE0 → romaji, kanji, english names
//	amask byte3 0xF0 → epno, ep name, ep romaji, ep kanji
//
// Index after split: 0=fid 1=aid 2=eid 3=size 4=ed2k 5=md5 6=total_eps
// 7=romaji 8=kanji 9=english 10=epno 11=ep_name 12=ep_romaji 13=ep_kanji.
const (
	fileFieldAID       = 1
	fileFieldEID       = 2
	fileFieldRomaji    = 7
	fileFieldKanji     = 8
	fileFieldEnglish   = 9
	fileFieldEpNo      = 10
	fileFieldEpName    = 11
	fileFieldEpRomaji  = 12
	fileFieldEpKanji   = 13
	fileFieldMinLen    = 14
	fileFieldAIDEidMin = 3
)

var (
	errIllegalInput   = fmt.Errorf("%w: illegal input", ErrRequestFailed)
	errInvalidSession = fmt.Errorf("%w: invalid session", ErrRequestFailed)
)

// MatchFileHash resolves identity via AniDB UDP FILE size+ed2k.
// Missing UDP creds or dial failures return MatchNone so the enricher falls back to title search.
func (p *Provider) MatchFileHash( //nolint:cyclop // soft-fallback branches for dial/creds/HTTP enrich
	ctx context.Context,
	hint provider.FileHashHint,
) (provider.MatchStatus, provider.FileHashFields, error) {
	user, pass := p.udpCreds()
	if user == "" || pass == "" || p.udpDialer == nil {
		return provider.MatchNone, provider.FileHashFields{}, nil
	}
	ed2k := strings.ToLower(strings.TrimSpace(hint.Ed2kHash))
	if ed2k == "" || hint.HashFileSize <= 0 {
		return provider.MatchNone, provider.FileHashFields{}, nil
	}
	err := p.ensureCreds()
	if err != nil {
		return provider.MatchNone, provider.FileHashFields{}, nil //nolint:nilerr // fall back to title search
	}

	hit, status, err := p.lookupFileByHash(ctx, hint.HashFileSize, ed2k)
	if err != nil {
		if provider.IsProviderAbort(err) {
			return provider.MatchNone, provider.FileHashFields{}, err
		}

		return provider.MatchNone, provider.FileHashFields{}, nil
	}
	if status != provider.MatchOK {
		return status, provider.FileHashFields{}, nil
	}

	fields, enrichErr := p.fileHashFields(ctx, hit)
	if enrichErr != nil {
		if provider.IsProviderAbort(enrichErr) {
			return provider.MatchNone, provider.FileHashFields{}, enrichErr
		}

		return provider.MatchOK, hit.toFields(), nil
	}

	return provider.MatchOK, fields, nil
}

type fileHit struct {
	aid, eid     string
	epNo         string
	animeTitle   string
	episodeTitle string
}

func (h fileHit) toFields() provider.FileHashFields {
	aid := strings.TrimSpace(h.aid)
	fields := provider.FileHashFields{IDs: provider.ExternalIDs{}}
	if aid != "" {
		fields.IDs.AnidbID = &aid
		fields.ExternalID = &aid
	}
	if title := strings.TrimSpace(h.animeTitle); title != "" {
		fields.Show = &title
		fields.Title = &title
	}
	if epTitle := strings.TrimSpace(h.episodeTitle); epTitle != "" {
		fields.EpisodeTitle = &epTitle
	}
	season, episode := parseAniDBEpNo(h.epNo)
	if season > 0 {
		fields.Season = &season
	}
	if episode > 0 {
		fields.Episode = &episode
	}

	return fields
}

func (p *Provider) fileHashFields(ctx context.Context, hit fileHit) (provider.FileHashFields, error) {
	fields := hit.toFields()
	if hit.aid == "" {
		return fields, nil
	}
	anime, err := p.getAnime(ctx, hit.aid)
	if err != nil {
		return fields, err
	}
	show := anime.toShowFields()
	fields.Show = show.Title
	fields.Title = show.Title
	fields.Description = show.Description
	fields.Genres = show.Genres
	fields.Year = show.Year
	fields.IDs = show.IDs
	fields.ExternalID = show.ExternalID
	mergeEpisodeFields(&fields, anime, hit)

	return fields, nil
}

func mergeEpisodeFields(fields *provider.FileHashFields, anime animeDTO, hit fileHit) {
	if fields.EpisodeTitle == nil || strings.TrimSpace(*fields.EpisodeTitle) == "" {
		if epTitle := episodeTitleByEIDOrEpNo(anime, hit.eid, hit.epNo); epTitle != "" {
			fields.EpisodeTitle = &epTitle
		}
	}
	season, episode := parseAniDBEpNo(hit.epNo)
	if season > 0 && fields.Season == nil {
		fields.Season = &season
	}
	if episode > 0 && fields.Episode == nil {
		fields.Episode = &episode
	}
}

func episodeTitleByEIDOrEpNo(anime animeDTO, eid, epNo string) string {
	eid = strings.TrimSpace(eid)
	epNo = strings.TrimSpace(epNo)
	for _, row := range anime.Episodes.Items {
		if eid != "" && strconv.Itoa(row.ID) == eid {
			if title := pickEpisodeTitle(row.Titles); title != "" {
				return title
			}
		}
		if epNo != "" && strings.EqualFold(strings.TrimSpace(row.EpNo.Value), epNo) {
			if title := pickEpisodeTitle(row.Titles); title != "" {
				return title
			}
		}
	}
	_, episode := parseAniDBEpNo(epNo)
	if episode <= 0 {
		return ""
	}
	mapped := mapEpisodes(anime.Episodes.Items)
	if info, ok := mapped[provider.EpisodeKey{Season: 1, Episode: episode}]; ok {
		return info.Title
	}

	return ""
}

func parseAniDBEpNo(raw string) (int, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw[0] < '0' || raw[0] > '9' {
		return 0, 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, 0
	}

	return 1, n
}

func (p *Provider) udpCreds() (string, string) {
	return strings.TrimSpace(p.username), p.password
}

func (p *Provider) lookupFileByHash(
	ctx context.Context,
	size int64,
	ed2k string,
) (fileHit, provider.MatchStatus, error) {
	p.udpMu.Lock()
	defer p.udpMu.Unlock()

	conn, remote, err := p.udpDialer.DialUDP(ctx, p.udpAddr)
	if err != nil {
		return fileHit{}, provider.MatchNone, nil
	}
	defer func() { _ = conn.Close() }()

	session, err := p.ensureUDPSessionLocked(ctx, conn, remote)
	if err != nil {
		return mapUDPErr(err)
	}

	hit, status, err := p.fileCommand(
		ctx, conn, remote, session, size, ed2k, fileFMaskFull, fileAMaskFull,
	)
	if errors.Is(err, errIllegalInput) {
		hit, status, err = p.fileCommand(
			ctx, conn, remote, session, size, ed2k, fileFMaskSimple, fileAMaskSimple,
		)
	}
	if errors.Is(err, errInvalidSession) {
		p.clearSessionLocked()
		session, authErr := p.authUDPLocked(ctx, conn, remote)
		if authErr != nil {
			return mapUDPErr(authErr)
		}
		hit, status, err = p.fileCommand(
			ctx, conn, remote, session, size, ed2k, fileFMaskSimple, fileAMaskSimple,
		)
	}
	if err != nil {
		if provider.IsProviderAbort(err) {
			p.clearSessionLocked()
		}

		return mapUDPErr(err)
	}

	return hit, status, nil
}

func mapUDPErr(err error) (fileHit, provider.MatchStatus, error) {
	if provider.IsProviderAbort(err) {
		return fileHit{}, provider.MatchNone, err
	}

	return fileHit{}, provider.MatchNone, nil
}

func (p *Provider) ensureUDPSessionLocked(
	ctx context.Context,
	conn net.PacketConn,
	remote net.Addr,
) (string, error) {
	if p.forceSession != "" {
		return p.forceSession, nil
	}
	ttl := p.sessionTTL
	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if p.sessionKey != "" && time.Since(p.sessionAt) < ttl {
		return p.sessionKey, nil
	}

	return p.authUDPLocked(ctx, conn, remote)
}

func (p *Provider) authUDPLocked(
	ctx context.Context,
	conn net.PacketConn,
	remote net.Addr,
) (string, error) {
	user, pass := p.udpCreds()
	params := url.Values{}
	params.Set("user", user)
	params.Set("pass", pass)
	params.Set("protover", udpProtoVer)
	params.Set("client", p.client)
	params.Set("clientver", udpClientVer)
	params.Set("enc", "UTF-8")
	params.Set("nat", "1")

	body, err := p.udpExchange(ctx, conn, remote, "AUTH "+params.Encode())
	if err != nil {
		return "", err
	}
	code, rest := splitUDPReply(body)
	switch code {
	case udpCodeLoginOK, udpCodeLoginNewVer:
		session := firstToken(rest)
		if session == "" {
			return "", fmt.Errorf("%w: empty session", ErrRequestFailed)
		}
		p.sessionKey = session
		p.sessionAt = time.Now()

		return session, nil
	case udpCodeLoginFailed, udpCodeIllegalInput:
		return "", fmt.Errorf("%w: auth %d", ErrRequestFailed, code)
	case udpCodeClientOutdated, udpCodeClientBanned, udpCodeBanned, udpCodeOutOfService:
		return "", udpUnavailable(code, rest)
	default:
		return "", fmt.Errorf(
			"%w: unexpected auth %d %s",
			ErrRequestFailed,
			code,
			truncate(rest),
		)
	}
}

func (p *Provider) clearSessionLocked() {
	p.sessionKey = ""
	p.sessionAt = time.Time{}
}

func (p *Provider) fileCommand(
	ctx context.Context,
	conn net.PacketConn,
	remote net.Addr,
	session string,
	size int64,
	ed2k, fmask, amask string,
) (fileHit, provider.MatchStatus, error) {
	params := url.Values{}
	params.Set("size", strconv.FormatInt(size, 10))
	params.Set("ed2k", ed2k)
	params.Set("fmask", fmask)
	params.Set("amask", amask)
	params.Set("s", session)

	body, err := p.udpExchange(ctx, conn, remote, "FILE "+params.Encode())
	if err != nil {
		return fileHit{}, provider.MatchNone, err
	}
	code, rest := splitUDPReply(body)
	switch code {
	case udpCodeFile:
		hit, parseErr := parseFILEReply(rest, fmask == fileFMaskFull)
		if parseErr != nil {
			return fileHit{}, provider.MatchNone, parseErr
		}

		return hit, provider.MatchOK, nil
	case udpCodeNoSuchFile:
		return fileHit{}, provider.MatchNone, nil
	case udpCodeIllegalInput:
		return fileHit{}, provider.MatchNone, fmt.Errorf(
			"%w: %s", errIllegalInput, truncate(rest),
		)
	case udpCodeLoginFirst, udpCodeInvalidSession:
		return fileHit{}, provider.MatchNone, fmt.Errorf("%w: %d", errInvalidSession, code)
	case udpCodeClientOutdated, udpCodeClientBanned, udpCodeBanned, udpCodeOutOfService:
		return fileHit{}, provider.MatchNone, udpUnavailable(code, rest)
	default:
		return fileHit{}, provider.MatchNone, fmt.Errorf(
			"%w: unexpected file %d %s",
			ErrRequestFailed,
			code,
			truncate(rest),
		)
	}
}

func udpUnavailable(code int, rest string) error {
	return fmt.Errorf(
		"%w: %w: udp %d %s",
		provider.ErrProviderUnavailable,
		ErrRequestFailed,
		code,
		truncate(rest),
	)
}

func parseFILEReply(rest string, rich bool) (fileHit, error) {
	fields := strings.Split(rest, "|")
	if len(fields) < fileFieldAIDEidMin {
		return fileHit{}, fmt.Errorf("%w: short FILE reply", ErrRequestFailed)
	}
	hit := fileHit{
		aid: strings.TrimSpace(fields[fileFieldAID]),
		eid: strings.TrimSpace(fields[fileFieldEID]),
	}
	if !rich || len(fields) < fileFieldMinLen {
		return hit, nil
	}
	hit.animeTitle = firstNonEmpty(
		strings.TrimSpace(fields[fileFieldEnglish]),
		strings.TrimSpace(fields[fileFieldRomaji]),
		strings.TrimSpace(fields[fileFieldKanji]),
	)
	hit.epNo = strings.TrimSpace(fields[fileFieldEpNo])
	hit.episodeTitle = firstNonEmpty(
		strings.TrimSpace(fields[fileFieldEpName]),
		strings.TrimSpace(fields[fileFieldEpRomaji]),
		strings.TrimSpace(fields[fileFieldEpKanji]),
	)

	return hit, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func (p *Provider) udpExchange(
	ctx context.Context,
	conn net.PacketConn,
	remote net.Addr,
	command string,
) (string, error) {
	p.http.Wait(ctx)
	err := ctx.Err()
	if err != nil {
		return "", fmt.Errorf("anidb udp canceled: %w", err)
	}

	payload := []byte(command)
	var lastErr error
	for attempt := 0; attempt <= udpMaxRetries; attempt++ {
		_ = conn.SetWriteDeadline(time.Now().Add(udpReadTimeout))
		_, err := conn.WriteTo(payload, remote)
		if err != nil {
			lastErr = fmt.Errorf("anidb udp write: %w", err)

			continue
		}
		_ = conn.SetReadDeadline(time.Now().Add(udpReadTimeout))
		buf := make([]byte, udpMaxPacket)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			lastErr = fmt.Errorf("anidb udp read: %w", err)

			continue
		}

		return strings.TrimSpace(string(buf[:n])), nil
	}

	return "", lastErr
}

func splitUDPReply(body string) (int, string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return 0, ""
	}
	parts := strings.SplitN(body, " ", udpReplySplitN)
	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, body
	}
	if len(parts) == 1 {
		return code, ""
	}

	return code, strings.TrimSpace(parts[1])
}

func firstToken(rest string) string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}
