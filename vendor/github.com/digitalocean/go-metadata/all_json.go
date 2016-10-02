package metadata

type Metadata struct {
	DropletID  int      `json:"droplet_id,omitempty"`
	Hostname   string   `json:"hostname,omitempty"`
	UserData   string   `json:"user_data,omitempty"`
	VendorData string   `json:"vendor_data,omitempty"`
	PublicKeys []string `json:"public_keys,omitempty"`
	Region     string   `json:"region,omitempty"`

	DNS struct {
		Nameservers []string `json:"nameservers,omitempty"`
	} `json:"dns,omitempty"`

	Interfaces map[string][]struct {
		MACAddress string `json:"mac,omitempty"`
		Type       string `json:"type,omitempty"`

		IPv4 *struct {
			IPAddress string `json:"ip_address,omitempty"`
			Netmask   string `json:"netmask,omitempty"`
			Gateway   string `json:"gateway,omitempty"`
		} `json:"ipv4,omitempty"`

		IPv6 *struct {
			IPAddress string `json:"ip_address,omitempty"`
			CIDR      int    `json:"cidr,omitempty"`
			Gateway   string `json:"gateway,omitempty"`
		} `json:"ipv6,omitempty"`

		AnchorIPv4 *struct {
			IPAddress string `json:"ip_address,omitempty"`
			Netmask   string `json:"netmask,omitempty"`
			Gateway   string `json:"gateway,omitempty"`
		} `json:"anchor_ipv4,omitempty"`
	} `json:"interfaces,omitempty"`

	FloatingIP struct {
		IPv4 struct {
			IPAddress string `json:"ip_address,omitempty"`
			Active    bool   `json:"active,omitempty"`
		} `json:"ipv4,omitempty"`
	} `json:"floating_ip",omitempty"`
}
