package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// dalang app: GitHub apps (a repository with a Dockerfile, built and run by Dalang, rebuilt
// by itself on every push). Everything the dashboard's "App Hosting dari GitHub" does, so an
// AI agent needs no dashboard. The one thing that cannot be done from here is linking
// GitHub to the account in the first place: that is an OAuth login in a browser.
//
// A GitHub app is stateless (rebuilt from the repository). Its environment variables are
// saved with 'app env' and used by the NEXT build: the running container is not touched, so a
// change is followed by a rebuild ('app env --rebuild').

const appCmdUsage = "usage: dalang app <list|info|repos|create|rebuild|env|logs|extend|settings|delete> ..."

func cmdApp(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printAppHelp()
		return nil
	}
	rest := args[1:]
	switch args[0] {
	case "list", "ls":
		return appList()
	case "info", "show":
		return appInfo(rest)
	case "repos":
		return appRepos(rest)
	case "create", "deploy":
		return appCreate(rest)
	case "rebuild", "redeploy":
		return appRebuild(rest)
	case "env":
		return appEnv(rest)
	case "logs":
		return appLogs(rest)
	case "extend", "subscribe", "renew":
		return appExtend(rest)
	case "settings":
		return appSettings(rest)
	case "delete", "rm":
		return appDelete(rest)
	}
	return fmt.Errorf("unknown app command %q (%s)", args[0], appCmdUsage)
}

// ── what the API returns ────────────────────────────────────────────────────

type appDetails struct {
	ID                 string            `json:"id"`
	RepoFullName       string            `json:"repo_full_name"`
	DisplayName        string            `json:"display_name"`
	Branch             string            `json:"branch"`
	Port               int               `json:"port"`
	Subdomain          string            `json:"subdomain"`
	AutoDeploy         bool              `json:"auto_deploy"`
	Status             string            `json:"status"`
	EnvVars            map[string]string `json:"env_vars"`
	ContainerName      string            `json:"container_name"`
	ExpiredAt          string            `json:"expired_at"`
	LastBuildAt        string            `json:"last_build_at"`
	Paused             bool              `json:"paused"`
	AutoRenew          bool              `json:"auto_renew"`
	CancelAtPeriodEnd  bool              `json:"cancel_at_period_end"`
	RenewalMonths      int               `json:"renewal_months"`
	RenewalAmountGross int64             `json:"renewal_amount_gross"`
	DestroyAt          string            `json:"destroy_at"`
}

func (d appDetails) name() string {
	if d.DisplayName != "" {
		return d.DisplayName
	}
	return d.RepoFullName
}

func (d appDetails) url() string { return appURL(d.Subdomain) }

func appURL(subdomain string) string {
	switch {
	case subdomain == "":
		return ""
	case strings.HasPrefix(subdomain, "http"):
		return subdomain
	}
	return "https://" + subdomain
}

func getAppDetails(client *api.Client, id string) (*appDetails, error) {
	resp, err := client.Get("/github/deployments/details?id=" + id)
	if err != nil {
		return nil, fmt.Errorf("could not read the app: %w", err)
	}
	var out struct {
		Data appDetails `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the app: %w", err)
	}
	return &out.Data, nil
}

func listApps(client *api.Client) ([]api.Deployment, error) {
	resp, err := client.Get("/github/deployments")
	if err != nil {
		return nil, fmt.Errorf("could not list apps: %w", err)
	}
	var out api.DeploymentListResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the app list: %w", err)
	}
	return out.Data, nil
}

// resolveApp turns a name (display name, repository, short repository name, subdomain) or an
// id into one app. Several matches is an error that lists the ids.
func resolveApp(client *api.Client, name string) (*api.Deployment, error) {
	apps, err := listApps(client)
	if err != nil {
		return nil, err
	}
	return matchApp(apps, name)
}

func matchApp(apps []api.Deployment, name string) (*api.Deployment, error) {
	cands := make([]shellCandidate, 0, len(apps))
	byID := map[string]api.Deployment{}
	for _, d := range apps {
		cands = append(cands, appCandidate(d))
		byID[d.ID] = d
	}
	matches := matchShellCandidates(cands, name)
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no app named '%s'. List yours with 'dalang app list'", name)
	case 1:
		d := byID[matches[0].ID]
		return &d, nil
	}
	var lines []string
	for _, m := range matches {
		lines = append(lines, fmt.Sprintf("  %s  %s  (%s)", m.ID, m.Name, m.Status))
	}
	return nil, fmt.Errorf("'%s' matches %d apps; use the id of the one you mean:\n%s", name, len(matches), strings.Join(lines, "\n"))
}

// waitForApp waits for a build to end: running is success, failed (or a stopped app) is not.
func waitForApp(client *api.Client, id string, timeout time.Duration) (*appDetails, error) {
	var last *appDetails
	err := waitFor(timeout, 10*time.Second, "the app to be running", func() (bool, error) {
		d, err := getAppDetails(client, id)
		if err != nil {
			return false, err
		}
		last = d
		switch strings.ToLower(d.Status) {
		case "running":
			return true, nil
		case "failed", "stopped":
			return false, fmt.Errorf("the app is %s. See why with 'dalang app logs %s --build'", d.Status, id)
		}
		say("  status: %s", d.Status)
		return false, nil
	})
	return last, err
}

// ── list / info / repos ─────────────────────────────────────────────────────

func appList() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	apps, err := listApps(client)
	if err != nil {
		return err
	}
	type row struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Repo      string `json:"repo"`
		Branch    string `json:"branch"`
		Status    string `json:"status"`
		URL       string `json:"url"`
		ExpiresAt string `json:"expires_at"`
	}
	rows := make([]row, 0, len(apps))
	for _, a := range apps {
		rows = append(rows, row{a.ID, deploymentName(a), a.RepoFullName, a.Branch, a.Status, appURL(a.Subdomain), a.ExpiresAt})
	}
	if jsonOutput {
		emitJSON(rows)
		return nil
	}
	if len(rows) == 0 {
		say("No apps. Deploy one with: dalang app create --repo owner/name")
		return nil
	}
	fmt.Printf("%-10s %-24s %-10s %s\n", "ID", "NAME", "STATUS", "URL")
	for _, r := range rows {
		fmt.Printf("%-10s %-24s %-10s %s\n", shortID(r.ID), truncate(r.Name, 24), r.Status, r.URL)
	}
	return nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func appInfo(args []string) error {
	pos, err := parseInterspersed(newFlags("app info"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app info <name|id>")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	d, err := getAppDetails(client, app.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{
			"id": d.ID, "name": d.name(), "repo": d.RepoFullName, "branch": d.Branch, "port": d.Port,
			"status": d.Status, "url": d.url(), "auto_deploy": d.AutoDeploy, "expires_at": d.ExpiredAt,
			"last_build_at": d.LastBuildAt, "paused": d.Paused, "auto_renew": d.AutoRenew,
			"cancel_at_period_end": d.CancelAtPeriodEnd, "renewal_months": d.RenewalMonths,
			"renewal_amount_gross": d.RenewalAmountGross, "destroy_at": d.DestroyAt,
			"env_vars": d.EnvVars, "container": d.ContainerName,
		})
		return nil
	}
	fmt.Printf("%s%s%s  (%s)\n", colorBold, d.name(), colorReset, d.ID)
	fmt.Printf("  Repo:         %s @ %s   (push to rebuild: %v)\n", d.RepoFullName, d.Branch, d.AutoDeploy)
	fmt.Printf("  Status:       %s%s\n", d.Status, map[bool]string{true: "  (paused: out of credits)"}[d.Paused])
	fmt.Printf("  URL:          %s   (port %d)\n", d.url(), d.Port)
	fmt.Printf("  Expires:      %s   auto-renew: %v   stop at period end: %v\n", d.ExpiredAt, d.AutoRenew, d.CancelAtPeriodEnd)
	if d.DestroyAt != "" {
		fmt.Printf("  Destroyed at: %s unless renewed\n", d.DestroyAt)
	}
	fmt.Printf("  Last build:   %s\n", d.LastBuildAt)
	if len(d.EnvVars) > 0 {
		keys := make([]string, 0, len(d.EnvVars))
		for k := range d.EnvVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("  Env:          %s\n", strings.Join(keys, ", "))
	}
	return nil
}

type ghRepo struct {
	FullName      string `json:"full_name"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
}

func githubConnected(client *api.Client) error {
	resp, err := client.Get("/github/status")
	if err != nil {
		return fmt.Errorf("could not check the GitHub connection: %w", err)
	}
	var out struct {
		Data struct {
			Connected bool `json:"connected"`
		} `json:"data"`
	}
	if json.Unmarshal(resp, &out) != nil || !out.Data.Connected {
		return fmt.Errorf("GitHub is not connected to this account. Connect it once in the dashboard (it is a browser login with GitHub); after that everything here works without it")
	}
	return nil
}

func listGitHubRepos(client *api.Client) ([]ghRepo, error) {
	if err := githubConnected(client); err != nil {
		return nil, err
	}
	resp, err := client.Get("/github/repos")
	if err != nil {
		return nil, fmt.Errorf("could not list repositories: %w", err)
	}
	var out struct {
		Data []ghRepo `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the repositories: %w", err)
	}
	return out.Data, nil
}

func appRepos(args []string) error {
	var search string
	fs := newFlags("app repos")
	fs.StringVar(&search, "search", "", "")
	if _, err := parseInterspersed(fs, args); err != nil {
		return err
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	repos, err := listGitHubRepos(client)
	if err != nil {
		return err
	}
	var shown []ghRepo
	for _, r := range repos {
		if search == "" || strings.Contains(strings.ToLower(r.FullName+" "+r.Description), strings.ToLower(search)) {
			shown = append(shown, r)
		}
	}
	if jsonOutput {
		emitJSON(shown)
		return nil
	}
	for _, r := range shown {
		vis := "public"
		if r.Private {
			vis = "private"
		}
		fmt.Printf("%-40s %-8s %s\n", r.FullName, vis, r.DefaultBranch)
	}
	return nil
}

// ── create ──────────────────────────────────────────────────────────────────

func appCreate(args []string) error {
	var repo, branch, name string
	var port, months int
	var maxPrice int64
	var timeout time.Duration
	wait := true
	var env kvFlag
	fs := newFlags("app create")
	fs.StringVar(&repo, "repo", "", "")
	fs.StringVar(&branch, "branch", "", "")
	fs.StringVar(&name, "name", "", "")
	fs.IntVar(&port, "port", 0, "")
	fs.IntVar(&months, "months", 1, "")
	fs.Int64Var(&maxPrice, "max-price", 0, "")
	fs.DurationVar(&timeout, "timeout", 20*time.Minute, "")
	fs.BoolVar(&wait, "wait", true, "")
	fs.Var(&env, "env", "")
	if pos, err := parseInterspersed(fs, args); err != nil {
		return err
	} else if len(pos) > 0 {
		return fmt.Errorf("unexpected argument %q. Usage: dalang app create --repo owner/name [--branch B] [--port N] [--env K=V]... [--months 1|12] --yes", pos[0])
	}
	owner, repoName, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || repoName == "" {
		return fmt.Errorf("--repo owner/name is required (see 'dalang app repos')")
	}
	if months != 1 && months != 12 {
		return fmt.Errorf("--months must be 1 or 12")
	}

	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput

	repos, err := listGitHubRepos(client)
	if err != nil {
		return err
	}
	var found *ghRepo
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, repo) {
			found = &repos[i]
		}
	}
	if found == nil {
		return fmt.Errorf("repository %s is not among the repositories your GitHub account can see ('dalang app repos')", repo)
	}
	if branch == "" {
		branch = found.DefaultBranch
		if branch == "" {
			branch = "main"
		}
	}

	// A repository without a Dockerfile in its root cannot be built: say so before anything is
	// created or charged.
	resp, err := client.Get(fmt.Sprintf("/github/repos/dockerfile?owner=%s&repo=%s&branch=%s", owner, repoName, branch))
	if err != nil {
		return fmt.Errorf("could not check for a Dockerfile: %w", err)
	}
	var df struct {
		Data struct {
			Exists      bool `json:"exists"`
			ExposedPort int  `json:"exposed_port"`
		} `json:"data"`
	}
	json.Unmarshal(resp, &df)
	if !df.Data.Exists {
		return fmt.Errorf("%s@%s has no Dockerfile in its root. A GitHub app is built from a Dockerfile (use an Alpine-based image, e.g. FROM node:22-alpine) and listens on the port it EXPOSEs", repo, branch)
	}
	if port == 0 {
		port = df.Data.ExposedPort
	}
	if port == 0 {
		return fmt.Errorf("the Dockerfile has no EXPOSE: pass --port N, the port your app listens on")
	}
	if name == "" {
		name = repoName
	}

	say("Deploying %s@%s on port %d (rebuilds on every push)", repo, branch, port)
	created, err := client.Post("/github/deployments/create", map[string]interface{}{
		"repo_owner": owner, "repo_name": repoName, "branch": branch, "port": port,
		"auto_deploy": true, "service_name": name, "env_vars": map[string]string(env),
	})
	if err != nil {
		return fmt.Errorf("could not create the deployment: %w", err)
	}
	var cr struct {
		Data struct {
			DeploymentID string `json:"deployment_id"`
		} `json:"data"`
	}
	json.Unmarshal(created, &cr)
	id := cr.Data.DeploymentID
	if id == "" {
		return fmt.Errorf("the deployment was created but the API did not return its id")
	}

	receipt, err := payWithQuote(client, subscriptionPayment(id, repo, months, maxPrice))
	if err != nil {
		return fmt.Errorf("%w\nThe app %s exists but is not paid for, so it has not been built. Pay with: dalang app extend %s --months %d --yes", err, id, id, months)
	}
	say("Paid. Building…")

	result := map[string]interface{}{"id": id, "name": name, "repo": repo, "branch": branch, "port": port, "paid_gross": receipt.Gross, "balance": receipt.NewBalance}
	if !wait {
		result["status"] = "building"
		return finishApp(result, nil)
	}
	d, err := waitForApp(client, id, timeout)
	if d != nil {
		result["status"], result["url"] = d.Status, d.url()
	}
	if err != nil {
		if jsonOutput {
			result["error"] = err.Error()
			emitJSON(result)
		}
		return err
	}
	return finishApp(result, d)
}

func finishApp(result map[string]interface{}, d *appDetails) error {
	if jsonOutput {
		emitJSON(result)
		return nil
	}
	printSuccess("%s is %v", result["name"], result["status"])
	if u, ok := result["url"].(string); ok && u != "" {
		fmt.Printf("  %s\n", u)
	}
	fmt.Printf("  id: %v   (push to %v rebuilds it)\n", result["id"], result["branch"])
	return nil
}

func subscriptionPayment(deploymentID, repo string, months int, maxGross int64) creditPayment {
	period := "bulanan"
	if months == 12 {
		period = "tahunan"
	}
	return creditPayment{
		ProductType: "deployment_subscription",
		Description: fmt.Sprintf("App Hosting %s - %s", period, repo),
		Metadata:    map[string]interface{}{"type": "deployment_subscription", "deployment_id": deploymentID, "billing_period": months},
		MaxGross:    maxGross,
	}
}

// ── rebuild / logs / extend / settings / delete ─────────────────────────────

func appRebuild(args []string) error {
	var timeout time.Duration
	wait := true
	fs := newFlags("app rebuild")
	fs.DurationVar(&timeout, "timeout", 20*time.Minute, "")
	fs.BoolVar(&wait, "wait", true, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app rebuild <name|id> [--wait=false] [--timeout 20m]")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	return runRebuild(client, app, wait, timeout, nil)
}

// runRebuild starts a build of the app's branch (which runs with the environment saved at that
// moment) and, with wait, follows it to running or failed. `extra` is added to the result.
func runRebuild(client *api.Client, app *api.Deployment, wait bool, timeout time.Duration, extra map[string]interface{}) error {
	say("Rebuilding %s from %s…", deploymentName(*app), app.Branch)
	if _, err := client.Post("/github/deployments/rebuild", map[string]string{"deployment_id": app.ID}); err != nil {
		return fmt.Errorf("could not start the rebuild: %w", err)
	}
	result := map[string]interface{}{"id": app.ID, "name": deploymentName(*app), "status": "building"}
	for k, v := range extra {
		result[k] = v
	}
	if !wait {
		return finishApp(result, nil)
	}
	d, err := waitForApp(client, app.ID, timeout)
	if d != nil {
		result["status"], result["url"], result["branch"] = d.Status, d.url(), d.Branch
	}
	if err != nil {
		if jsonOutput {
			result["error"] = err.Error()
			emitJSON(result)
		}
		return err
	}
	return finishApp(result, d)
}

// appEnv shows or changes the environment of an app. The API REPLACES the whole map, so a
// change is merged into what is stored first; it is used from the next build.
func appEnv(args []string) error {
	var set kvFlag
	var unset stringList
	var rebuild, reveal bool
	var timeout time.Duration
	wait := true
	fs := newFlags("app env")
	fs.Var(&set, "set", "")
	fs.Var(&unset, "unset", "")
	fs.BoolVar(&rebuild, "rebuild", false, "")
	fs.BoolVar(&reveal, "reveal", false, "")
	fs.BoolVar(&wait, "wait", true, "")
	fs.DurationVar(&timeout, "timeout", 20*time.Minute, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app env <name|id> [--set KEY=VALUE]... [--unset KEY]... [--rebuild] [--reveal]")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	d, err := getAppDetails(client, app.ID)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for k, v := range d.EnvVars {
		env[k] = v
	}

	if len(set) == 0 && len(unset) == 0 {
		if rebuild {
			return runRebuild(client, app, wait, timeout, nil) // rebuild with what is saved
		}
		if jsonOutput {
			emitJSON(env)
			return nil
		}
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if reveal {
				fmt.Printf("%s=%s\n", k, env[k])
			} else {
				fmt.Printf("%s=********\n", k)
			}
		}
		return nil
	}

	for k, v := range set {
		env[k] = v
	}
	for _, k := range unset {
		delete(env, k)
	}
	if _, err := client.Post("/github/deployments/update-env", map[string]interface{}{"deployment_id": app.ID, "env_vars": env}); err != nil {
		return fmt.Errorf("could not save the environment: %w", err)
	}
	say("Saved %d variable(s). They are used from the next build.", len(env))
	if rebuild {
		return runRebuild(client, app, wait, timeout, map[string]interface{}{"env_keys": sortedKeys(env)})
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": app.ID, "env_keys": sortedKeys(env), "applies_on": "next_build"})
		return nil
	}
	say("Apply it now with: dalang app rebuild %s", deploymentName(*app))
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func appLogs(args []string) error {
	var build bool
	var lines int
	fs := newFlags("app logs")
	fs.BoolVar(&build, "build", false, "")
	fs.IntVar(&lines, "lines", 100, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app logs <name|id> [--build] [--lines N]")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	if build {
		resp, err := client.Get("/github/deployments/logs?id=" + app.ID)
		if err != nil {
			return fmt.Errorf("could not read the build log: %w", err)
		}
		var out struct {
			Data []struct {
				ID            string `json:"id"`
				Status        string `json:"status"`
				StartedAt     string `json:"started_at"`
				CommitSHA     string `json:"commit_sha"`
				CommitMessage string `json:"commit_message"`
				Logs          string `json:"logs"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp, &out); err != nil {
			return fmt.Errorf("could not read the build log: %w", err)
		}
		if jsonOutput {
			emitJSON(out.Data)
			return nil
		}
		if len(out.Data) == 0 {
			say("No builds yet.")
			return nil
		}
		b := out.Data[0] // newest first
		fmt.Fprintf(stderrOrStdout(), "# build %s  %s  %s  %s\n", b.ID, b.Status, shortID(b.CommitSHA), b.CommitMessage)
		fmt.Print(b.Logs)
		return nil
	}
	resp, err := client.Get(fmt.Sprintf("/github/deployments/container-logs?id=%s&lines=%d", app.ID, lines))
	if err != nil {
		return fmt.Errorf("could not read the runtime log: %w", err)
	}
	var out struct {
		Data struct {
			Logs string `json:"logs"`
		} `json:"data"`
	}
	json.Unmarshal(resp, &out)
	if jsonOutput {
		emitJSON(map[string]string{"logs": out.Data.Logs})
		return nil
	}
	fmt.Print(out.Data.Logs)
	return nil
}

func appExtend(args []string) error {
	var months int
	var maxPrice int64
	fs := newFlags("app extend")
	fs.IntVar(&months, "months", 1, "")
	fs.Int64Var(&maxPrice, "max-price", 0, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app extend <name|id> [--months 1|12] [--max-price N] --yes")
	}
	if months != 1 && months != 12 {
		return fmt.Errorf("--months must be 1 or 12")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	receipt, err := payWithQuote(client, subscriptionPayment(app.ID, app.RepoFullName, months, maxPrice))
	if err != nil {
		return err
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": app.ID, "months": months, "paid_gross": receipt.Gross, "balance": receipt.NewBalance})
		return nil
	}
	printSuccess("%s extended by %d month(s). Credits left: %s", deploymentName(*app), months, formatIDR(receipt.NewBalance))
	return nil
}

func appSettings(args []string) error {
	var autoRenew, stopAtEnd string
	fs := newFlags("app settings")
	fs.StringVar(&autoRenew, "auto-renew", "", "")
	fs.StringVar(&stopAtEnd, "stop-at-period-end", "", "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app settings <name|id> [--auto-renew on|off] [--stop-at-period-end on|off]")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	body := map[string]interface{}{"deployment_id": app.ID}
	for flagName, val := range map[string]string{"auto_renew": autoRenew, "cancel_at_period_end": stopAtEnd} {
		if val == "" {
			continue
		}
		b, err := parseOnOff(val)
		if err != nil {
			return err
		}
		body[flagName] = b
	}
	if len(body) == 1 {
		return appInfo([]string{app.ID}) // nothing to change: show the current settings
	}
	if _, err := client.Post("/github/deployments/subscription-settings", body); err != nil {
		return fmt.Errorf("could not change the settings: %w", err)
	}
	d, err := getAppDetails(client, app.ID)
	if err != nil {
		return err
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": d.ID, "auto_renew": d.AutoRenew, "cancel_at_period_end": d.CancelAtPeriodEnd, "expires_at": d.ExpiredAt})
		return nil
	}
	printSuccess("auto-renew: %v   stop at period end: %v   (until %s)", d.AutoRenew, d.CancelAtPeriodEnd, d.ExpiredAt)
	return nil
}

func parseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("expected on or off, got %q", v)
}

func appDelete(args []string) error {
	pos, err := parseInterspersed(newFlags("app delete"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang app delete <name|id> --yes")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	app, err := resolveApp(client, pos[0])
	if err != nil {
		return err
	}
	if err := requireConfirmation(fmt.Sprintf("Delete app %s permanently? Its container is destroyed and the remaining subscription is not refunded", deploymentName(*app))); err != nil {
		return err
	}
	if _, err := client.Post("/github/deployments/delete", map[string]string{"deployment_id": app.ID}); err != nil {
		return fmt.Errorf("could not delete the app: %w", err)
	}
	if jsonOutput {
		emitJSON(map[string]interface{}{"id": app.ID, "deleted": true})
		return nil
	}
	printSuccess("%s deleted", deploymentName(*app))
	return nil
}

func printAppHelp() {
	fmt.Printf(`%sdalang app%s - Deploy and manage GitHub apps (a repo with a Dockerfile)

%sUSAGE:%s
    dalang app list                                  Your apps
    dalang app info <name|id>                        Status, URL, subscription
    dalang app repos [--search text]                 Repositories you can deploy
    dalang app create --repo owner/name [flags]      Deploy a repository (pays from credits)
    dalang app rebuild <name|id>                     Build the branch again
    dalang app env <name|id> [--set K=V]... [--unset K]... [--rebuild]   Show or change the environment
    dalang app logs <name|id> [--build] [--lines N]  Runtime log, or the latest build log
    dalang app extend <name|id> [--months 1|12]      Pay for another period
    dalang app settings <name|id> [--auto-renew on|off] [--stop-at-period-end on|off]
    dalang app delete <name|id>                      Destroy the app

%sCREATE FLAGS:%s
    --repo owner/name     the repository (required)
    --branch B            default: the repository's default branch
    --port N              default: the Dockerfile's EXPOSE
    --env KEY=VALUE       repeatable; change it later with 'dalang app env'.
    --name text           display name, default: the repository name
    --months 1|12         subscription period (default 1)
    --max-price N         refuse to pay a total (incl. PPN) above N rupiah
    --wait=false          return once the build has started instead of when it is running
    --timeout 20m         how long to wait for the build

%sAUTO-REDEPLOY:%s
    An app is rebuilt by itself on every push to its branch (a webhook on the repository).
    'dalang app rebuild' builds it again without a push.

%sENVIRONMENT:%s
    'dalang app env shop --set API_KEY=... --unset OLD --rebuild' saves the change and builds again.
    Saved variables are used by the NEXT build; the running container is not touched until then.
    Without --set/--unset it lists the variables (values hidden; --reveal shows them, --json always does).

%sREQUIREMENTS:%s
    - A file named Dockerfile in the root of the repository, with an Alpine-based image
      (FROM node:22-alpine ...). A Debian or Ubuntu base builds but cannot start.
    - The app listens on the port the Dockerfile EXPOSEs. It is stateless: files written
      in the container are lost on each build; keep data elsewhere.
    - GitHub must be connected to the account once (a browser login, in the dashboard).
    - Enough credits: the price is quoted by the API and shown before it is charged.

%sFOR SCRIPTS AND AI AGENTS:%s
    --json     one JSON document on stdout; progress goes to stderr
    --yes      required to pay or delete when there is no terminal to ask
    exit code  0 on success, 1 on any failure (message on stderr)

%sEXAMPLES:%s
    dalang app create --repo acme/shop --env PORT=3000 --months 1 --yes --json
    dalang app logs shop --build
    dalang app settings shop --auto-renew off
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
