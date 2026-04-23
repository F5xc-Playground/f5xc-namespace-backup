package registry

var allResources = []Resource{
	// ── Tier 1: Standalone Primitives ─────────────────────────────────────────

	// Certificates
	{Kind: "certificate", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/certificates"},
	{Kind: "certificate-chain", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/certificate_chains"},
	{Kind: "trusted-ca-list", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/trusted_ca_lists"},
	{Kind: "crl", Domain: "certificates", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/crls"},

	// Network primitives
	{Kind: "ip-prefix-set", Domain: "network", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/ip_prefix_sets"},
	{Kind: "geo-location-set", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/geo_location_sets"},

	// Healthchecks
	{Kind: "healthcheck", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/healthchecks"},

	// Rate limiting primitives
	{Kind: "rate-limiter", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/rate_limiters"},
	{Kind: "policer", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/policers"},
	{Kind: "protocol-policer", Domain: "rate_limiting", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/protocol_policers"},

	// User identification
	{Kind: "user-identification", Domain: "tenant_and_identity", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/user_identifications"},

	// App firewall (WAF)
	{Kind: "app-firewall", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_firewalls"},

	// Data types
	{Kind: "data-type", Domain: "data_and_privacy_security", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/data_types"},

	// BIG-IP
	{Kind: "bigip-irule", Domain: "bigip", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/bigip_irules"},
	{Kind: "data-group", Domain: "bigip", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/data_groups"},

	// Virtual site
	{Kind: "virtual-site", Domain: "sites", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/virtual_sites"},

	// Virtual network
	{Kind: "virtual-network", Domain: "network", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/virtual_networks"},

	// Cloud credentials
	{Kind: "cloud-credentials", Domain: "cloud_infrastructure", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/cloud_credentialss"},

	// Secrets & blindfold
	{Kind: "secret-policy", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_policys"},
	{Kind: "secret-policy-rule", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_policy_rules"},
	{Kind: "secret-management-access", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/secret_management_accesss"},
	{Kind: "voltshare-admin-policy", Domain: "blindfold", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/voltshare_admin_policys"},

	// Observability
	{Kind: "alert-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/alert_receivers"},
	{Kind: "alert-policy", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/alert_policys"},
	{Kind: "global-log-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/global_log_receivers"},
	{Kind: "log-receiver", Domain: "statistics", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/log_receivers"},

	// Malicious user mitigation
	{Kind: "malicious-user-mitigation", Domain: "secops_and_incident_response", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/malicious_user_mitigations"},

	// Protocol inspection
	{Kind: "protocol-inspection", Domain: "virtual", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/protocol_inspections"},

	// Workload flavors
	{Kind: "workload-flavor", Domain: "container_services", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/workload_flavors"},

	// K8s policies
	{Kind: "k8s-cluster-role", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_cluster_roles"},
	{Kind: "k8s-pod-security-admission", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_pod_security_admissions"},
	{Kind: "k8s-pod-security-policy", Domain: "managed_kubernetes", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/k8s_pod_security_policys"},

	// DDoS
	{Kind: "infraprotect-asn-prefix", Domain: "ddos", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_asn_prefixs"},
	{Kind: "infraprotect-firewall-rule", Domain: "ddos", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_firewall_rules"},

	// App setting
	{Kind: "app-setting", Domain: "service_mesh", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_settings"},
	{Kind: "app-type", Domain: "service_mesh", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/app_types"},

	// NGINX
	{Kind: "nginx-service-discovery", Domain: "nginx_one", Tier: 1,
		ListPath: "/api/config/namespaces/{namespace}/nginx_service_discoverys"},

	// ── Tier 2: Reference tier-1 objects ──────────────────────────────────────

	// Origin pool (references healthcheck)
	{Kind: "origin-pool", Domain: "virtual", Tier: 2, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/origin_pools"},

	// Security policies
	{Kind: "service-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/service_policys"},
	{Kind: "service-policy-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/service_policy_rules"},
	{Kind: "network-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_policys"},
	{Kind: "network-policy-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_rules"},
	{Kind: "network-firewall", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_firewalls"},
	{Kind: "fast-acl-rule", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/fast_acl_rules"},
	{Kind: "filter-set", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/filter_sets"},
	{Kind: "segment", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/segments"},
	{Kind: "nat-policy", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/nat_policys"},

	// Rate limiter policy
	{Kind: "rate-limiter-policy", Domain: "virtual", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/rate_limiter_policys"},

	// WAF exclusion policy
	{Kind: "waf-exclusion-policy", Domain: "virtual", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/waf_exclusion_policys"},

	// Sensitive data policy
	{Kind: "sensitive-data-policy", Domain: "data_and_privacy_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/sensitive_data_policys"},

	// Network routing
	{Kind: "route", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/routes"},
	{Kind: "network-connector", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/network_connectors"},
	{Kind: "bgp-routing-policy", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/bgp_routing_policys"},
	{Kind: "advertise-policy", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/advertise_policys"},
	{Kind: "srv6-network-slice", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/srv6_network_slices"},
	{Kind: "dc-cluster-group", Domain: "network", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/dc_cluster_groups"},
	{Kind: "policy-based-routing", Domain: "network_security", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/policy_based_routings"},

	// K8s cluster role binding (refs cluster role)
	{Kind: "k8s-cluster-role-binding", Domain: "managed_kubernetes", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/k8s_cluster_role_bindings"},

	// DDoS groups
	{Kind: "infraprotect-firewall-rule-group", Domain: "ddos", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_firewall_rule_groups"},
	{Kind: "infraprotect-internet-prefix-advertisement", Domain: "ddos", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/infraprotect_internet_prefix_advertisements"},

	// API definition
	{Kind: "api-definition", Domain: "api", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/api_definitions"},
	{Kind: "app-api-group", Domain: "api", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/app_api_groups"},

	// Marketplace
	{Kind: "external-connector", Domain: "marketplace", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/external_connectors"},
	{Kind: "addon-subscription", Domain: "marketplace", Tier: 2,
		ListPath: "/api/config/namespaces/{namespace}/addon_subscriptions"},

	// ── Tier 3: Higher-level policy views ─────────────────────────────────────

	// Network policy view (creates network_policy + rules)
	{Kind: "network-policy-view", Domain: "network_security", Tier: 3, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_views"},

	// Forward proxy policy (creates service_policy + rules)
	{Kind: "forward-proxy-policy", Domain: "network_security", Tier: 3, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/forward_proxy_policys"},

	// Fast ACL (refs fast-acl-rule)
	{Kind: "fast-acl", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/fast_acls"},

	// Enhanced firewall policy
	{Kind: "enhanced-firewall-policy", Domain: "virtual", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/enhanced_firewall_policys"},

	// Service/network policy sets (read-only; view-owned instances filtered by owner_view at fetch time)
	{Kind: "service-policy-set", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/service_policy_sets"},
	{Kind: "network-policy-set", Domain: "network_security", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/network_policy_sets"},

	// DNS resources (note: different API prefix!)
	{Kind: "dns-zone", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_zones"},
	{Kind: "dns-domain", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_domains"},
	{Kind: "dns-lb-health-check", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_lb_health_checks"},
	{Kind: "dns-lb-pool", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_lb_pools"},
	{Kind: "dns-load-balancer", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_load_balancers"},
	{Kind: "dns-compliance-checks", Domain: "dns", Tier: 3,
		ListPath: "/api/config/dns/namespaces/{namespace}/dns_compliance_checkss"},

	// API security
	{Kind: "api-discovery", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_discoverys"},
	{Kind: "api-crawler", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_crawlers"},
	{Kind: "api-testing", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/api_testings"},
	{Kind: "discovery", Domain: "api", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/discoverys"},

	// Bot defense
	{Kind: "bot-defense-app-infrastructure", Domain: "bot_and_threat_defense", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/bot_defense_app_infrastructures"},
	{Kind: "protected-application", Domain: "shape", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/protected_applications"},

	// Service mesh
	{Kind: "site-mesh-group", Domain: "service_mesh", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/site_mesh_groups"},
	// Endpoint (view-owned instances filtered by owner_view at fetch time)
	{Kind: "endpoint", Domain: "service_mesh", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/endpoints"},

	// Cloud
	{Kind: "cloud-connect", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_connects"},
	{Kind: "cloud-elastic-ip", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_elastic_ips"},
	{Kind: "cloud-link", Domain: "cloud_infrastructure", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/cloud_links"},

	// CE management
	{Kind: "network-interface", Domain: "ce_management", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/network_interfaces"},
	{Kind: "usb-policy", Domain: "ce_management", Tier: 3,
		ListPath: "/api/config/namespaces/{namespace}/usb_policys"},

	// ── Tier 4: Top-level view objects (LBs, etc.) ────────────────────────────

	// Load balancers
	{Kind: "http-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/http_loadbalancers"},
	{Kind: "tcp-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/tcp_loadbalancers"},
	{Kind: "udp-loadbalancer", Domain: "virtual", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/udp_loadbalancers"},
	{Kind: "cdn-loadbalancer", Domain: "cdn", Tier: 4, IsView: true,
		ListPath: "/api/config/namespaces/{namespace}/cdn_loadbalancers"},

	// Container services
	{Kind: "virtual-k8s", Domain: "container_services", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/virtual_k8ss"},
	{Kind: "workload", Domain: "container_services", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/workloads"},
	{Kind: "k8s-cluster", Domain: "sites", Tier: 4,
		ListPath: "/api/config/namespaces/{namespace}/k8s_clusters"},

	// ── Tier 5: View-managed children (skipped in smart mode) ─────────────────

	{Kind: "virtual-host", Domain: "virtual", Tier: 5, ManagedBy: "http-loadbalancer",
		ListPath: "/api/config/namespaces/{namespace}/virtual_hosts"},
	{Kind: "cluster", Domain: "virtual", Tier: 5, ManagedBy: "origin-pool",
		ListPath: "/api/config/namespaces/{namespace}/clusters"},
	{Kind: "proxy", Domain: "virtual", Tier: 5, ManagedBy: "http-loadbalancer",
		ListPath: "/api/config/namespaces/{namespace}/proxys"},
}
