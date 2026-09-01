package domain

import (
	"regexp"
	"strings"

	"stickyproxy/native-plugin/internal/i18n"
)

// providerAdapter is the private seam for all platform-specific behavior.
// Proxy creation, account synchronization, and server-side testing cross
// this same seam, so a provider's validation and session syntax stay local.
type providerAdapter interface {
	Validate(Parts) error
	NormalizeBase(Parts) (Parts, error)
	RequiresAccountIdentity() bool
	RequiresTestSession() bool
	RewriteAccount(parts Parts, stableHash string) (RewriteResult, error)
	RewriteTest(parts Parts, randomSession string) (RewriteResult, error)
}

var (
	dataImpulseSession = regexp.MustCompile(`(?i);sessid\.[^;:@]*`)
	// Some existing URLs contain a legacy Decodo token with hyphens. Remove
	// the complete session/sessionduration pair before injecting the canonical
	// hash, rather than leaving a second marker in the username.
	decodoSession = regexp.MustCompile(`(?i)-session-[A-Za-z0-9_-]+-sessionduration-\d+|-sessionduration-\d+|-session-[A-Za-z0-9_-]+`)
	proxy1024SID  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	providers     = map[Platform]providerAdapter{
		DataImpulse: dataImpulseAdapter{},
		Decodo:      decodoAdapter{},
		Resin:       resinAdapter{},
		Proxy1024:   proxy1024Adapter{},
		Generic:     genericAdapter{},
	}
)

func providerFor(platform Platform) (providerAdapter, bool) {
	adapter, ok := providers[platform]
	return adapter, ok
}

type dataImpulseAdapter struct{}

func (dataImpulseAdapter) Validate(parts Parts) error {
	if parts.Username == "" {
		return i18n.New("missing_username", "DataImpulse")
	}
	return nil
}

func (dataImpulseAdapter) NormalizeBase(parts Parts) (Parts, error) { return parts, nil }

func (dataImpulseAdapter) RequiresAccountIdentity() bool { return true }

func (dataImpulseAdapter) RequiresTestSession() bool { return true }

func (dataImpulseAdapter) RewriteAccount(parts Parts, stableHash string) (RewriteResult, error) {
	username := dataImpulseSession.ReplaceAllString(parts.Username, "") + ";sessid." + stableHash
	return RewriteResult{URL: Build(parts, username, parts.Password), StableHash: stableHash}, nil
}

func (dataImpulseAdapter) RewriteTest(parts Parts, randomSession string) (RewriteResult, error) {
	username := dataImpulseSession.ReplaceAllString(parts.Username, "") + ";sessid." + randomSession
	return RewriteResult{URL: Build(parts, username, parts.Password)}, nil
}

type decodoAdapter struct{}

func (decodoAdapter) Validate(parts Parts) error {
	if strings.EqualFold(parts.Host, "gate.decodo.com") && parts.Username == "" {
		return i18n.New("missing_username", "Decodo")
	}
	return nil
}

func (decodoAdapter) NormalizeBase(parts Parts) (Parts, error) { return parts, nil }

func (decodoAdapter) RequiresAccountIdentity() bool { return true }

func (decodoAdapter) RequiresTestSession() bool { return true }

func (decodoAdapter) RewriteAccount(parts Parts, stableHash string) (RewriteResult, error) {
	if !strings.EqualFold(parts.Host, "gate.decodo.com") {
		return RewriteResult{URL: Build(parts, parts.Username, parts.Password), StableHash: stableHash, PortStickyOnly: true}, nil
	}
	username := decodoSessionUsername(parts.Username, stableHash)
	return RewriteResult{URL: Build(parts, username, parts.Password), StableHash: stableHash}, nil
}

func (decodoAdapter) RewriteTest(parts Parts, randomSession string) (RewriteResult, error) {
	if !strings.EqualFold(parts.Host, "gate.decodo.com") {
		return RewriteResult{URL: Build(parts, parts.Username, parts.Password), PortStickyOnly: true}, nil
	}
	username := decodoSessionUsername(parts.Username, randomSession)
	return RewriteResult{URL: Build(parts, username, parts.Password)}, nil
}

func decodoSessionUsername(username, session string) string {
	base := decodoSession.ReplaceAllString(username, "")
	if !strings.HasPrefix(strings.ToLower(base), "user-") {
		base = "user-" + base
	}
	return base + "-session-" + session + "-sessionduration-30"
}

type resinAdapter struct{}

func (resinAdapter) Validate(parts Parts) error {
	if parts.Password == "" {
		return i18n.New("missing_resin_token")
	}
	return nil
}

func (resinAdapter) NormalizeBase(parts Parts) (Parts, error) { return parts, nil }

func (resinAdapter) RequiresAccountIdentity() bool { return true }

func (resinAdapter) RequiresTestSession() bool { return true }

func (resinAdapter) RewriteAccount(parts Parts, stableHash string) (RewriteResult, error) {
	// Resin owns the sticky lease. Account is a non-reversible account hash,
	// so it does not expose the email in Proxy-Authorization.
	return RewriteResult{
		URL:        Build(parts, "Default.sp_"+stableHash, parts.Password),
		StableHash: stableHash,
	}, nil
}

func (resinAdapter) RewriteTest(parts Parts, randomSession string) (RewriteResult, error) {
	return RewriteResult{URL: Build(parts, "Default.test_"+randomSession, parts.Password)}, nil
}

type proxy1024Adapter struct{}

func (proxy1024Adapter) Validate(parts Parts) error {
	if parts.Username == "" {
		return i18n.New("missing_username", "1024Proxy")
	}
	if parts.Password == "" {
		return i18n.New("missing_proxy_password", "1024Proxy")
	}
	_, err := parse1024Username(parts.Username)
	return err
}

func (proxy1024Adapter) NormalizeBase(parts Parts) (Parts, error) { return parts, nil }

func (proxy1024Adapter) RequiresAccountIdentity() bool { return true }

func (proxy1024Adapter) RequiresTestSession() bool { return true }

func (proxy1024Adapter) RewriteAccount(parts Parts, stableHash string) (RewriteResult, error) {
	username, err := rewrite1024Username(parts.Username, stableHash)
	if err != nil {
		return RewriteResult{}, err
	}
	return RewriteResult{URL: Build(parts, username, parts.Password), StableHash: stableHash}, nil
}

func (proxy1024Adapter) RewriteTest(parts Parts, randomSession string) (RewriteResult, error) {
	username, err := rewrite1024Username(parts.Username, randomSession)
	if err != nil {
		return RewriteResult{}, err
	}
	return RewriteResult{URL: Build(parts, username, parts.Password)}, nil
}

// parsed1024Username represents the only syntax that StickyProxy owns: one
// complete -sid-<id> marker. Any final -t-... suffix belongs to 1024Proxy and
// remains opaque and byte-for-byte unchanged.
type parsed1024Username struct {
	base        string
	templateSID string
	ttlSuffix   string
}

func parse1024Username(username string) (parsed1024Username, error) {
	if username == "" {
		return parsed1024Username{}, i18n.New("invalid_1024_sid")
	}

	const marker = "-sid-"
	if strings.Count(username, marker) > 1 {
		return parsed1024Username{}, i18n.New("invalid_1024_sid")
	}
	markerStart := strings.Index(username, marker)
	if markerStart < 0 {
		base, ttl := splitTerminal1024TTL(username)
		if base == "" {
			return parsed1024Username{}, i18n.New("invalid_1024_sid")
		}
		return parsed1024Username{base: base, ttlSuffix: ttl}, nil
	}

	base := username[:markerStart]
	remainder := username[markerStart+len(marker):]
	if base == "" || remainder == "" {
		return parsed1024Username{}, i18n.New("invalid_1024_sid")
	}
	sessionID, ttl := splitTerminal1024TTL(remainder)
	if sessionID == "" || !proxy1024SID.MatchString(sessionID) {
		return parsed1024Username{}, i18n.New("invalid_1024_sid")
	}
	return parsed1024Username{base: base, templateSID: sessionID, ttlSuffix: ttl}, nil
}

func splitTerminal1024TTL(value string) (base, ttlSuffix string) {
	if start := strings.LastIndex(value, "-t-"); start >= 0 {
		return value[:start], value[start:]
	}
	return value, ""
}

func rewrite1024Username(username, sid string) (string, error) {
	if !proxy1024SID.MatchString(sid) {
		return "", i18n.New("invalid_1024_sid")
	}
	parsed, err := parse1024Username(username)
	if err != nil {
		return "", err
	}
	return parsed.base + "-sid-" + sid + parsed.ttlSuffix, nil
}

// genericAdapter preserves a direct proxy URL without inferring any upstream
// session or stickiness behavior from its host, port, or credentials.
type genericAdapter struct{}

func (genericAdapter) Validate(Parts) error { return nil }

func (genericAdapter) NormalizeBase(parts Parts) (Parts, error) { return parts, nil }

func (genericAdapter) RequiresAccountIdentity() bool { return false }

func (genericAdapter) RequiresTestSession() bool { return false }

func (genericAdapter) RewriteAccount(parts Parts, _ string) (RewriteResult, error) {
	return RewriteResult{URL: Build(parts, parts.Username, parts.Password)}, nil
}

func (genericAdapter) RewriteTest(parts Parts, _ string) (RewriteResult, error) {
	return RewriteResult{URL: Build(parts, parts.Username, parts.Password)}, nil
}
