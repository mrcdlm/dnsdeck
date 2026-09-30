package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mrcdlm/dnsdeck/internal/i18n"
)

type TunnelConnection struct {
	ID                 string    `json:"id"`
	ColoName           string    `json:"colo_name"`
	ClientID           string    `json:"client_id"`
	ClientVersion      string    `json:"client_version"`
	OpenedAt           time.Time `json:"opened_at"`
	OriginIP           string    `json:"origin_ip"`
	IsPendingReconnect bool      `json:"is_pending_reconnect"`
}

type Tunnel struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Status          string             `json:"status"` // inactive | degraded | healthy | down
	CreatedAt       time.Time          `json:"created_at"`
	ConnsActiveAt   *time.Time         `json:"conns_active_at"`
	ConnsInactiveAt *time.Time         `json:"conns_inactive_at"`
	TunType         string             `json:"tun_type"`
	RemoteConfig    bool               `json:"remote_config"`
	Connections     []TunnelConnection `json:"connections"`
}

// ErrTunnelPermission: dem Token fehlt Account → Cloudflare Tunnel → Read
// oder die Account-ID stimmt nicht.
var ErrTunnelPermission = errors.New("no access to tunnels")

// ListTunnels liefert alle nicht gelöschten Tunnels des Accounts.
func (c *Client) ListTunnels(ctx context.Context, accountID string) ([]Tunnel, error) {
	var out []Tunnel
	for page := 1; ; page++ {
		q := url.Values{"is_deleted": {"false"}, "per_page": {"50"}, "page": {fmt.Sprint(page)}}
		env, err := c.do(ctx, http.MethodGet, "/accounts/"+url.PathEscape(accountID)+"/cfd_tunnel", q, nil)
		if err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) && (apiErr.Status == http.StatusForbidden || apiErr.Status == http.StatusUnauthorized) {
				return nil, &i18n.Error{Wrap: ErrTunnelPermission,
					Msg: i18n.M("cf.tunnel_permission", "detail", i18n.Nest(apiErr.LocalizedMsg()))}
			}
			return nil, err
		}
		var tunnels []Tunnel
		if err := json.Unmarshal(env.Result, &tunnels); err != nil {
			return nil, err
		}
		for _, t := range tunnels {
			if t.Connections == nil {
				t.Connections = []TunnelConnection{}
			}
			out = append(out, t)
		}
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages || len(tunnels) == 0 {
			return out, nil
		}
	}
}
