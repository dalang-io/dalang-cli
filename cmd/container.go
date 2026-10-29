package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// dalang container: databases, ready-made apps and runtimes (PostgreSQL, Redis, WordPress,
// n8n, Node.js ...) from templates. Everything the dashboard's "Aplikasi Siap Pakai" does.
//
// A container is paid from credits at the plan's price; the PRICE is never here, it comes from
// POST /credits/quote (see credits_pay.go). What IS here is the template table: which types
// exist, their default version and the settings that must be given. It is a copy of
// dalang.io/src/lib/components/wizard/containerTemplates.js, and like any copy it can fall
// behind: a new template or version is accepted by passing --version / --type the API knows.

type containerTemplate struct {
	Type     string
	Version  string
	MustSet  []string // environment the template cannot start without and has no default for
	Needs    string   // "postgres" | "mysql": a database it is bundled with
	Optional string   // "postgres": may be linked to one
}

var containerTemplates = []containerTemplate{
	{"postgresql", "18.6-alpine", []string{"POSTGRES_PASSWORD"}, "", ""},
	{"mysql", "8.4.13", []string{"MYSQL_ROOT_PASSWORD"}, "", ""},
	{"mariadb", "11.8.9", []string{"MARIADB_ROOT_PASSWORD"}, "", ""},
	{"mongodb", "8.0.34", []string{"MONGO_INITDB_ROOT_PASSWORD"}, "", ""},
	{"redis", "8.8.4-alpine", nil, "", ""},
	{"rabbitmq", "4.4.0-management-alpine", []string{"RABBITMQ_DEFAULT_PASS"}, "", ""},
	{"n8n", "3.0.1", []string{"N8N_BASIC_AUTH_PASSWORD"}, "", "postgres"},
	{"gitea", "28.2.0", nil, "", ""},
	{"forgejo", "17.0.1", nil, "", ""},
	{"portainer", "2.45.3", nil, "", ""},
	{"uptime-kuma", "2.5.8", nil, "", ""},
	{"pgadmin", "9.18.0", []string{"PGADMIN_DEFAULT_PASSWORD"}, "", "postgres"},
	{"odoo", "20.0-20261015", nil, "postgres", ""},
	{"wordpress", "7.1.3", nil, "mysql", ""},
	{"php", "8.5.12-apache", nil, "", ""},
	{"nodejs", "24.22.1-alpine", nil, "", ""},
	{"python", "3.14.10-alpine", nil, "", ""},
	{"java", "25_36-jre", nil, "", ""},
	{"openclaw", "2026.3.1", []string{"OPENCLAW_GATEWAY_TOKEN"}, "", ""},
	{"hermes", "v0.22.5", []string{"HERMES_DASHBOARD_BASIC_AUTH_PASSWORD"}, "", ""},
}

// The plans the platform sells to containers (names only; the price is the API's).
var containerPlans = []string{"starter", "basic", "standard", "premium"}

func findTemplate(t string) *containerTemplate {
	for i := range containerTemplates {
		if containerTemplates[i].Type == t {
			return &containerTemplates[i]
		}
	}
	return nil
}

const containerCmdUsage = "usage: dalang container <templates|list|info|create|start|stop|restart|env|recreate> ..."

func cmdContainer(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printContainerHelp()
		return nil
	}
	rest := args[1:]
	switch args[0] {
	case "templates":
		return containerTemplatesList()
	case "list", "ls":
		return containerList()
	case "info", "show":
		return containerInfo(rest)
	case "create", "new":
		return containerCreate(rest)
	case "start", "stop", "restart":
		return containerAction(args[0], rest)
	case "env":
		return containerEnv(rest)
	case "recreate":
		return containerRecreate(rest)
	}
	return fmt.Errorf("unknown container command %q (%s)", args[0], containerCmdUsage)
}

func containerTemplatesList() error {
	if jsonOutput {
		type row struct {
			Type     string   `json:"type"`
			Version  string   `json:"default_version"`
			MustSet  []string `json:"must_set"`
			Needs    string   `json:"needs,omitempty"`
			Optional string   `json:"optional_link,omitempty"`
		}
		rows := make([]row, 0, len(containerTemplates))
		for _, t := range containerTemplates {
			rows = append(rows, row{t.Type, t.Version, append([]string{}, t.MustSet...), t.Needs, t.Optional})
		}
		emitJSON(map[string]interface{}{"templates": rows, "plans": containerPlans})
		return nil
	}
	fmt.Printf("%-12s %-26s %s\n", "TYPE", "DEFAULT VERSION", "MUST SET (--env)")
	for _, t := range containerTemplates {
		extra := strings.Join(t.MustSet, ", ")
		switch {
		case t.Needs != "":
			extra += map[bool]string{true: "; "}[extra != ""] + "bundled " + t.Needs + " (--create-" + t.Needs + " or --" + t.Needs + "-id)"
		case t.Optional != "":
			extra += map[bool]string{true: "; "}[extra != ""] + "may link a " + t.Optional
		}
		fmt.Printf("%-12s %-26s %s\n", t.Type, t.Version, extra)
	}
	fmt.Printf("\nPlans: %s. The price is quoted by the API before anything is charged.\n", strings.Join(containerPlans, ", "))
	return nil
}

// ── reading ─────────────────────────────────────────────────────────────────

func listContainers(client *api.Client) ([]api.Container, error) {
	resp, err := client.Get("/containers/list")
	if err != nil {
		return nil, fmt.Errorf("could not list containers: %w", err)
	}
	var out api.ContainerListResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the container list: %w", err)
	}
	return out.Data.Containers, nil
}

func matchContainer(list []api.Container, name string) (*api.Container, error) {
	cands := make([]shellCandidate, 0, len(list))
	byID := map[string]api.Container{}
	for _, c := range list {
		cands = append(cands, containerCandidate(c))
		byID[c.ID] = c
	}
	matches := matchShellCandidates(cands, name)
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no container named '%s'. List yours with 'dalang container list'", name)
	case 1:
		c := byID[matches[0].ID]
		return &c, nil
	}
	var lines []string
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("  %s  %s  (%s)", m.ID, m.Name, m.Status))
	}
	return nil, fmt.Errorf("'%s' matches %d containers; use the id of the one you mean:\n%s", name, len(matches), strings.Join(lines, "\n"))
}

func resolveContainer(client *api.Client, name string) (*api.Container, error) {
	list, err := listContainers(client)
	if err != nil {
		return nil, err
	}
	return matchContainer(list, name)
}

func containerList() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	list, err := listContainers(client)
	if err != nil {
		return err
	}
	type row struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		Plan      string `json:"plan"`
		Status    string `json:"status"`
		ExpiresAt string `json:"expires_at"`
	}
	rows := make([]row, 0, len(list))
	for _, c := range list {
		rows = append(rows, row{c.ID, containerName(c), c.ContainerType, c.Plan, c.Status, c.ExpiresAt})
	}
	if jsonOutput {
		emitJSON(rows)
		return nil
	}
	if len(rows) == 0 {
		say("No containers. Create one with: dalang container create --type redis")
		return nil
	}
	fmt.Printf("%-10s %-22s %-12s %-9s %s\n", "ID", "NAME", "TYPE", "PLAN", "STATUS")
	for _, r := range rows {
		fmt.Printf("%-10s %-22s %-12s %-9s %s\n", shortID(r.ID), truncate(r.Name, 22), r.Type, r.Plan, r.Status)
	}
	return nil
}

type containerDetail struct {
	Container struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		ContainerType string `json:"container_type"`
		Version       string `json:"container_version"`
		Plan          string `json:"plan"`
		Status        string `json:"status"`
		ExpiredAt     string `json:"expired_at"`
	} `json:"container"`
	Detail struct {
		Domain           string          `json:"domain"`
		IPv4Local        string          `json:"ipv4_local"`
		Port             int             `json:"port"`
		ConnectionString string          `json:"connection_string"`
		EnvVars          json.RawMessage `json:"env_vars"`
	} `json:"detail"`
}

func getContainerDetail(client *api.Client, id string) (*containerDetail, error) {
	resp, err := client.Get("/containers/detail?id=" + id)
	if err != nil {
		return nil, fmt.Errorf("could not read the container: %w", err)
	}
	var out struct {
		Data containerDetail `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the container: %w", err)
	}
	return &out.Data, nil
}

// envOf reads the stored environment: the API sends it as a JSON string (or, in time, an object).
func (d *containerDetail) env() map[string]string {
	out := map[string]string{}
	raw := d.Detail.EnvVars
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		raw = json.RawMessage(asString)
	}
	json.Unmarshal(raw, &out)
	return out
}

func containerInfo(args []string) error {
	pos, err := parseInterspersed(newFlags("container info"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang container info <name|id>")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	c, err := resolveContainer(client, pos[0])
	if err != nil {
		return err
	}
	d, err := getContainerDetail(client, c.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{
			"id": d.Container.ID, "name": containerName(*c), "type": d.Container.ContainerType, "version": d.Container.Version,
			"plan": d.Container.Plan, "status": d.Container.Status, "expires_at": d.Container.ExpiredAt,
			"domain": d.Detail.Domain, "host": d.Detail.IPv4Local, "port": d.Detail.Port,
			"connection_string": d.Detail.ConnectionString, "env": d.env(),
		})
		return nil
	}
	fmt.Printf("%s%s%s  (%s)\n", colorBold, containerName(*c), colorReset, d.Container.ID)
	fmt.Printf("  Type:       %s %s   plan %s\n", d.Container.ContainerType, d.Container.Version, d.Container.Plan)
	fmt.Printf("  Status:     %s   expires %s\n", d.Container.Status, d.Container.ExpiredAt)
	if d.Detail.Domain != "" {
		fmt.Printf("  Domain:     %s\n", d.Detail.Domain)
	}
	if d.Detail.ConnectionString != "" {
		fmt.Printf("  Connect:    %s\n", d.Detail.ConnectionString)
	}
	keys := []string{}
	for k := range d.env() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		fmt.Printf("  Env:        %s\n", strings.Join(keys, ", "))
	}
	return nil
}

// ── create ──────────────────────────────────────────────────────────────────

func containerCreate(args []string) error {
	var typ, version, plan, name, mysqlEngine, pgID, mysqlID, pgPass, mysqlPass string
	var createPG, createMySQL bool
	var maxPrice int64
	var timeout time.Duration
	wait := true
	var env kvFlag
	fs := newFlags("container create")
	fs.StringVar(&typ, "type", "", "")
	fs.StringVar(&version, "version", "", "")
	fs.StringVar(&plan, "plan", "starter", "")
	fs.StringVar(&name, "name", "", "")
	fs.Var(&env, "env", "")
	fs.StringVar(&pgID, "postgres-id", "", "")
	fs.BoolVar(&createPG, "create-postgres", false, "")
	fs.StringVar(&pgPass, "postgres-password", "", "")
	fs.StringVar(&mysqlID, "mysql-id", "", "")
	fs.BoolVar(&createMySQL, "create-mysql", false, "")
	fs.StringVar(&mysqlPass, "mysql-password", "", "")
	fs.StringVar(&mysqlEngine, "mysql-engine", "mariadb", "")
	fs.Int64Var(&maxPrice, "max-price", 0, "")
	fs.DurationVar(&timeout, "timeout", 15*time.Minute, "")
	fs.BoolVar(&wait, "wait", true, "")
	if pos, err := parseInterspersed(fs, args); err != nil {
		return err
	} else if len(pos) > 0 {
		return fmt.Errorf("unexpected argument %q. Usage: dalang container create --type T [--plan P] [--env K=V]... --yes (see 'dalang container templates')", pos[0])
	}

	req, err := buildContainerRequest(typ, version, plan, name, env, pgID, createPG, pgPass, mysqlID, createMySQL, mysqlPass, mysqlEngine)
	if err != nil {
		return err
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	before, err := listContainers(client)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, c := range before {
		known[c.ID] = true
	}

	label := req["service_name"].(string)
	receipt, err := payWithQuote(client, creditPayment{
		ProductType: "container",
		Description: fmt.Sprintf("%s (%s) - %s Plan", typ, req["container_version"], strings.ToUpper(plan[:1])+plan[1:]),
		Metadata:    req,
		MaxGross:    maxPrice,
	})
	if err != nil {
		return err
	}
	say("Paid. Creating %s…", label)

	result := map[string]interface{}{"name": label, "type": typ, "plan": plan, "paid_gross": receipt.Gross, "balance": receipt.NewBalance}
	if !wait {
		result["status"] = "creating"
		return finishContainer(result)
	}
	// The container appears when the payment has been fulfilled: look for the one that is new.
	var created *api.Container
	err = waitFor(timeout, 5*time.Second, "the container to appear", func() (bool, error) {
		list, err := listContainers(client)
		if err != nil {
			return false, err
		}
		for i := range list {
			if !known[list[i].ID] && (containerName(list[i]) == label || list[i].Name == label) {
				created = &list[i]
				return strings.EqualFold(list[i].Status, "running"), nil
			}
		}
		return false, nil
	})
	if created != nil {
		result["id"], result["status"] = created.ID, created.Status
	}
	if err != nil {
		if jsonOutput {
			result["error"] = err.Error()
			emitJSON(result)
		}
		return fmt.Errorf("%w. It was paid for: check 'dalang container list'", err)
	}
	return finishContainer(result)
}

func finishContainer(result map[string]interface{}) error {
	if jsonOutput {
		emitJSON(result)
		return nil
	}
	printSuccess("%v is %v", result["name"], result["status"])
	if id, ok := result["id"]; ok {
		fmt.Printf("  id: %v   (connection details: dalang container info %v)\n", id, result["name"])
	}
	return nil
}

// buildContainerRequest validates what a container needs and builds the metadata the payment
// carries. Everything it checks is something the dashboard would not let you leave out.
func buildContainerRequest(typ, version, plan, name string, env kvFlag, pgID string, createPG bool, pgPass, mysqlID string, createMySQL bool, mysqlPass, mysqlEngine string) (map[string]interface{}, error) {
	t := findTemplate(typ)
	if t == nil {
		return nil, fmt.Errorf("unknown --type %q. See 'dalang container templates'", typ)
	}
	validPlan := false
	for _, p := range containerPlans {
		validPlan = validPlan || p == plan
	}
	if !validPlan {
		return nil, fmt.Errorf("--plan must be one of %s", strings.Join(containerPlans, ", "))
	}
	if version == "" {
		version = t.Version
	}
	for _, key := range t.MustSet {
		if _, ok := env[key]; !ok {
			return nil, fmt.Errorf("%s needs --env %s=... (it cannot start without it)", typ, key)
		}
	}
	if name == "" {
		name = fmt.Sprintf("%s-%d", typ, time.Now().Unix()%100000)
	}
	if env == nil {
		env = kvFlag{}
	}
	req := map[string]interface{}{
		"product_type":      "container",
		"container_type":    typ,
		"container_version": version,
		"container_plan":    plan,
		"container_env":     map[string]string(env),
		"service_name":      name,
	}

	switch {
	case t.Needs == "postgres" || (t.Optional == "postgres" && (pgID != "" || createPG)):
		switch {
		case pgID != "":
			req["linked_postgres_id"] = pgID
		case createPG && pgPass != "":
			req["create_linked_postgres"] = true
			req["postgres_password"] = pgPass
		case t.Needs == "postgres":
			return nil, fmt.Errorf("%s runs on a PostgreSQL: pass --postgres-id <id> to use one you have, or --create-postgres --postgres-password P to get one with it", typ)
		default:
			return nil, fmt.Errorf("--create-postgres needs --postgres-password")
		}
	case t.Needs == "mysql":
		switch {
		case mysqlID != "":
			req["linked_mysql_id"] = mysqlID
		case createMySQL && mysqlPass != "":
			req["create_linked_mysql"] = true
			req["mysql_password"] = mysqlPass
			req["mysql_type"] = mysqlEngine
		default:
			return nil, fmt.Errorf("%s runs on a MySQL/MariaDB: pass --mysql-id <id>, or --create-mysql --mysql-password P [--mysql-engine mariadb|mysql]", typ)
		}
	}
	return req, nil
}

// ── start / stop / restart / env / recreate ─────────────────────────────────

func containerAction(action string, args []string) error {
	pos, err := parseInterspersed(newFlags("container "+action), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang container %s <name|id>", action)
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	c, err := resolveContainer(client, pos[0])
	if err != nil {
		return err
	}
	if _, err := client.Post("/containers/action", map[string]string{"container_id": c.ID, "action": action}); err != nil {
		return fmt.Errorf("could not %s the container: %w", action, err)
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": c.ID, "name": containerName(*c), "action": action, "ok": true})
		return nil
	}
	printSuccess("%s: %s", containerName(*c), action)
	return nil
}

// containerEnv shows the environment, or changes it. The API REPLACES the whole map (a key left
// out is removed), so a change is merged into what is there first; it applies after a restart.
func containerEnv(args []string) error {
	var set kvFlag
	var unset stringList
	var restart bool
	fs := newFlags("container env")
	fs.Var(&set, "set", "")
	fs.Var(&unset, "unset", "")
	fs.BoolVar(&restart, "restart", false, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang container env <name|id> [--set K=V]... [--unset K]... [--restart]")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	c, err := resolveContainer(client, pos[0])
	if err != nil {
		return err
	}
	d, err := getContainerDetail(client, c.ID)
	if err != nil {
		return err
	}
	env := d.env()
	if len(set) == 0 && len(unset) == 0 {
		if jsonOutput {
			emitJSON(env)
			return nil
		}
		keys := []string{}
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s=%s\n", k, env[k])
		}
		return nil
	}
	for k, v := range set {
		env[k] = v
	}
	for _, k := range unset {
		delete(env, k)
	}
	if _, err := client.Post("/containers/update-env", map[string]interface{}{"container_id": c.ID, "env_vars": env}); err != nil {
		return fmt.Errorf("could not change the environment: %w", err)
	}
	if restart {
		if err := containerAction("restart", []string{c.ID}); err != nil {
			return err
		}
	} else {
		say("Saved. It takes effect after a restart: dalang container restart %s", containerName(*c))
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": c.ID, "env": env, "restarted": restart})
	}
	return nil
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func containerRecreate(args []string) error {
	pos, err := parseInterspersed(newFlags("container recreate"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang container recreate <name|id> --yes")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	c, err := resolveContainer(client, pos[0])
	if err != nil {
		return err
	}
	if err := requireConfirmation(fmt.Sprintf("Recreate %s from its image? Data outside its persistent volumes is lost", containerName(*c))); err != nil {
		return err
	}
	if _, err := client.Post("/containers/recreate", map[string]string{"container_id": c.ID}); err != nil {
		return fmt.Errorf("could not recreate the container: %w", err)
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": c.ID, "recreated": true})
		return nil
	}
	printSuccess("%s is being recreated", containerName(*c))
	return nil
}

func printContainerHelp() {
	fmt.Printf(`%sdalang container%s - Databases, ready-made apps and runtimes from templates

%sUSAGE:%s
    dalang container templates                       Types, default versions, what must be set
    dalang container list                            Your containers
    dalang container info <name|id>                  Status, address, connection string, env keys
    dalang container create --type T [flags]         Create one (pays from credits)
    dalang container start|stop|restart <name|id>
    dalang container env <name|id> [--set K=V]... [--unset K]... [--restart]
    dalang container recreate <name|id>              Rebuild from the image (non-persistent data is lost)

%sCREATE FLAGS:%s
    --type T                 see 'dalang container templates' (required)
    --plan starter|basic|standard|premium     (default starter)
    --version V              default: the template's default version
    --name text              default: type-<number>
    --env KEY=VALUE          repeatable; the template's required settings must be given
    --postgres-id ID | --create-postgres --postgres-password P      (odoo needs one; n8n, pgAdmin may)
    --mysql-id ID | --create-mysql --mysql-password P [--mysql-engine mariadb|mysql]   (wordpress needs one)
    --max-price N            refuse a total (incl. PPN) above N rupiah
    --wait=false             return once paid instead of when it is running
    --timeout 15m

%sFOR SCRIPTS AND AI AGENTS:%s
    --json     one JSON document on stdout; progress goes to stderr
    --yes      required to pay or recreate when there is no terminal to ask
    The price is quoted by the API and shown before it is charged; 'dalang credit' shows and tops up credits.
    A shell inside a container: 'dalang shell <name>'. Env changes apply after a restart.

%sEXAMPLES:%s
    dalang container create --type postgresql --name orders-db --env POSTGRES_PASSWORD=s3cret --yes --json
    dalang container create --type wordpress --create-mysql --mysql-password s3cret --yes
    dalang container env orders-db --set MAX_CONNECTIONS=200 --restart
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
