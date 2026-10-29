package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// dalang volume: extra storage that can be attached to a VPS. Priced and charged by the API
// when it is created (credits); nothing here works out a price.
//
// The fields of a volume are read tolerantly (a map, not a fixed struct) and --json passes the
// API's own document through, so a field the API adds is never lost on the way to an agent.

type volumeRec map[string]interface{}

func (v volumeRec) str(key string) string {
	if s, ok := v[key].(string); ok {
		return s
	}
	if n, ok := v[key].(float64); ok {
		return fmt.Sprintf("%g", n)
	}
	return ""
}

const volumeCmdUsage = "usage: dalang volume <list|create|attach|detach|resize|delete> ..."

func cmdVolume(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printVolumeHelp()
		return nil
	}
	rest := args[1:]
	switch args[0] {
	case "list", "ls":
		return volumeList()
	case "create", "new":
		return volumeCreate(rest)
	case "attach":
		return volumeAttach(rest)
	case "detach":
		return volumeDetach(rest)
	case "resize":
		return volumeResize(rest)
	case "delete", "rm":
		return volumeDelete(rest)
	}
	return fmt.Errorf("unknown volume command %q (%s)", args[0], volumeCmdUsage)
}

func listVolumes(client *api.Client) ([]volumeRec, error) {
	resp, err := client.Get("/volumes/list")
	if err != nil {
		return nil, fmt.Errorf("could not list volumes: %w", err)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, fmt.Errorf("could not read the volume list: %w", err)
	}
	var vols []volumeRec
	if json.Unmarshal(out.Data, &vols) != nil {
		var wrapped struct {
			Volumes []volumeRec `json:"volumes"`
		}
		json.Unmarshal(out.Data, &wrapped)
		vols = wrapped.Volumes
	}
	return vols, nil
}

// matchVolume finds a volume by id or by name; two with one name are refused with their ids.
func matchVolume(vols []volumeRec, ref string) (volumeRec, error) {
	var hits []volumeRec
	for _, fold := range []bool{false, true} {
		for _, v := range vols {
			for _, key := range []string{"id", "name", "display_name"} {
				val := v.str(key)
				if val != "" && (val == ref || (fold && strings.EqualFold(val, ref))) {
					hits = append(hits, v)
					break
				}
			}
		}
		if len(hits) > 0 {
			break
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("no volume '%s'. List yours with 'dalang volume list'", ref)
	case 1:
		return hits[0], nil
	}
	var lines []string
	for _, h := range hits {
		lines = append(lines, fmt.Sprintf("  %s  %s", h.str("id"), h.str("name")))
	}
	return nil, fmt.Errorf("'%s' matches %d volumes; use the id of the one you mean:\n%s", ref, len(hits), strings.Join(lines, "\n"))
}

func resolveVolume(client *api.Client, ref string) (volumeRec, error) {
	vols, err := listVolumes(client)
	if err != nil {
		return nil, err
	}
	return matchVolume(vols, ref)
}

func volumeList() error {
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	vols, err := listVolumes(client)
	if err != nil {
		return err
	}
	if jsonOutput {
		if vols == nil {
			vols = []volumeRec{}
		}
		emitJSON(vols)
		return nil
	}
	if len(vols) == 0 {
		say("No volumes. Create one with: dalang volume create --name data --size 10 --yes")
		return nil
	}
	fmt.Printf("%-10s %-22s %-8s %s\n", "ID", "NAME", "SIZE GB", "STATUS")
	for _, v := range vols {
		fmt.Printf("%-10s %-22s %-8s %s\n", shortID(v.str("id")), truncate(v.str("name"), 22), v.str("size_gb"), v.str("status"))
	}
	return nil
}

func volumeCreate(args []string) error {
	var name string
	var size int
	fs := newFlags("volume create")
	fs.StringVar(&name, "name", "", "")
	fs.IntVar(&size, "size", 0, "")
	if pos, err := parseInterspersed(fs, args); err != nil || len(pos) > 0 {
		return fmt.Errorf("usage: dalang volume create --name N --size GB --yes")
	}
	if name == "" || size <= 0 {
		return fmt.Errorf("--name and --size (GB, whole number) are required")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	if err := requireConfirmation(fmt.Sprintf("Create a %d GB volume '%s'? It is charged from your credits at the platform's price", size, name)); err != nil {
		return err
	}
	resp, err := client.Post("/volumes/create", map[string]interface{}{"name": name, "size_gb": size, "pay_method": "credits"})
	if err != nil {
		return fmt.Errorf("could not create the volume: %w", err)
	}
	return printAPIResult(resp, fmt.Sprintf("volume %s created", name))
}

// printAPIResult prints the API's own `data` under --json, a one-line confirmation otherwise.
func printAPIResult(resp []byte, human string) error {
	if jsonOutput {
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		json.Unmarshal(resp, &out)
		if len(out.Data) == 0 {
			out.Data = json.RawMessage(`{"ok":true}`)
		}
		fmt.Println(string(out.Data))
		return nil
	}
	printSuccess("%s", human)
	return nil
}

func volumeAttach(args []string) error {
	var vpsName string
	fs := newFlags("volume attach")
	fs.StringVar(&vpsName, "vps", "", "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 || vpsName == "" {
		return fmt.Errorf("usage: dalang volume attach <volume> --vps <vps-name>")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	v, err := resolveVolume(client, pos[0])
	if err != nil {
		return err
	}
	vps, err := findVPSByName(client, vpsName)
	if err != nil {
		return err
	}
	resp, err := client.Post("/volumes/attach", map[string]string{"volume_id": v.str("id"), "vps_id": vps.ID})
	if err != nil {
		return fmt.Errorf("could not attach the volume: %w", err)
	}
	return printAPIResult(resp, fmt.Sprintf("%s attached to %s", v.str("name"), vpsName))
}

func volumeDetach(args []string) error {
	pos, err := parseInterspersed(newFlags("volume detach"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang volume detach <volume>")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	v, err := resolveVolume(client, pos[0])
	if err != nil {
		return err
	}
	resp, err := client.Post("/volumes/detach", map[string]string{"volume_id": v.str("id")})
	if err != nil {
		return fmt.Errorf("could not detach the volume: %w", err)
	}
	return printAPIResult(resp, fmt.Sprintf("%s detached", v.str("name")))
}

func volumeResize(args []string) error {
	var size int
	fs := newFlags("volume resize")
	fs.IntVar(&size, "size", 0, "")
	pos, err := parseInterspersed(fs, args)
	if err != nil || len(pos) != 1 || size <= 0 {
		return fmt.Errorf("usage: dalang volume resize <volume> --size GB --yes")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	v, err := resolveVolume(client, pos[0])
	if err != nil {
		return err
	}
	if err := requireConfirmation(fmt.Sprintf("Resize %s to %d GB? A larger size is charged from your credits", v.str("name"), size)); err != nil {
		return err
	}
	resp, err := client.Post("/volumes/resize", map[string]interface{}{"volume_id": v.str("id"), "new_size_gb": size})
	if err != nil {
		return fmt.Errorf("could not resize the volume: %w", err)
	}
	return printAPIResult(resp, fmt.Sprintf("%s resized to %d GB", v.str("name"), size))
}

func volumeDelete(args []string) error {
	pos, err := parseInterspersed(newFlags("volume delete"), args)
	if err != nil || len(pos) != 1 {
		return fmt.Errorf("usage: dalang volume delete <volume> --yes")
	}
	client, err := api.NewAuthenticatedClient()
	if err != nil {
		return err
	}
	client.Verbose = VerboseOutput
	v, err := resolveVolume(client, pos[0])
	if err != nil {
		return err
	}
	if err := requireConfirmation(fmt.Sprintf("Delete volume %s and everything on it, permanently?", v.str("name"))); err != nil {
		return err
	}
	resp, err := client.Post("/volumes/delete", map[string]string{"volume_id": v.str("id")})
	if err != nil {
		return fmt.Errorf("could not delete the volume: %w", err)
	}
	return printAPIResult(resp, fmt.Sprintf("%s deleted", v.str("name")))
}

func printVolumeHelp() {
	fmt.Printf(`%sdalang volume%s - Extra storage for a VPS

%sUSAGE:%s
    dalang volume list
    dalang volume create --name N --size GB          Charged from credits by the API
    dalang volume attach <volume> --vps <vps-name>
    dalang volume detach <volume>
    dalang volume resize <volume> --size GB
    dalang volume delete <volume>                    Destroys the data on it

%sNOTES:%s
    <volume> is an id or a name. create, resize and delete need --yes when there is no
    terminal to ask. --json prints the API's own document on stdout.
    A container's persistent storage is not a volume you create here: it is attached to the
    container by the platform (see 'dalang container').
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
