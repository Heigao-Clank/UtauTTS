package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"utautts/internal/frontend"
	"utautts/internal/voicebank"
)

func main() {
	var (
		path        string
		listOnly    bool
		alias       string
		kana        string
		tone        string
		aliasPolicy string
		limit       int
	)

	flag.StringVar(&path, "oto", "", "path to a voicebank directory or oto.ini")
	flag.BoolVar(&listOnly, "list", false, "list aliases")
	flag.StringVar(&alias, "alias", "", "filter by alias")
	flag.StringVar(&kana, "kana", "", "resolve a kana reading and print selected aliases")
	flag.StringVar(&tone, "tone", "C4", "voicebank tone used with prefix.map")
	flag.StringVar(&aliasPolicy, "alias-policy", string(voicebank.AliasPolicyAuto), "voicebank alias policy: auto, vcv-prefer, or cv-only")
	flag.IntVar(&limit, "limit", 20, "maximum entries to show (0 means all)")
	flag.Parse()

	if path == "" {
		log.Fatal("-oto is required")
	}
	bank, err := voicebank.Load(path)
	if err != nil {
		log.Fatal(err)
	}

	if listOnly {
		for _, item := range bank.Aliases() {
			fmt.Println(item)
		}
		return
	}
	if alias != "" {
		entries := bank.Entries[alias]
		if len(entries) == 0 {
			log.Fatalf("alias not found: %s", alias)
		}
		if limit > 0 && len(entries) > limit {
			entries = entries[:limit]
		}
		for _, entry := range entries {
			fmt.Printf("%s=%s,%.3f,%.3f,%.3f,%.3f,%.3f\n",
				entry.Filename,
				entry.Alias,
				entry.Offset,
				entry.Fixed,
				entry.Blank,
				entry.Preutterance,
				entry.Overlap,
			)
		}
		return
	}
	if kana != "" {
		morae, err := frontend.ParseKana(kana)
		if err != nil {
			log.Fatal(err)
		}
		selections, err := bank.ResolveWithConfig(morae, voicebank.ResolveConfig{
			Tone: tone, AliasPolicy: voicebank.AliasPolicy(aliasPolicy),
		})
		if err != nil {
			log.Fatal(err)
		}
		for _, selection := range selections {
			fmt.Printf("%d\t%s\t%s\t%s\n", selection.Position, selection.Mora.Text, selection.Alias, selection.Entry.Filename)
		}
		return
	}

	fmt.Printf("voicebank=%s\n", bank.Name)
	fmt.Printf("root=%s\n", bank.Root)
	fmt.Printf("oto_files=%d\n", len(bank.OtoFiles))
	fmt.Printf("entries=%d\n", bank.EntryCount())
	fmt.Printf("aliases=%d\n", len(bank.Entries))
	capabilities := bank.AliasCapabilities()
	aliasCounts := capabilities.Counts
	fmt.Printf("alias_cv=%d\n", aliasCounts[voicebank.AliasCV])
	fmt.Printf("alias_vcv=%d\n", aliasCounts[voicebank.AliasVCV])
	fmt.Printf("alias_vc=%d\n", aliasCounts[voicebank.AliasVC])
	fmt.Printf("alias_other=%d\n", aliasCounts[voicebank.AliasOther])
	fmt.Printf("vcv_has_initial=%t\n", capabilities.HasInitialVCV)
	fmt.Printf("vcv_has_n_context=%t\n", capabilities.HasNContextVCV)
	contexts := make([]string, 0, len(capabilities.VCVContexts))
	for context := range capabilities.VCVContexts {
		contexts = append(contexts, context)
	}
	sort.Strings(contexts)
	for _, context := range contexts {
		fmt.Printf("vcv_context_%s=%d\n", context, capabilities.VCVContexts[context])
	}
	fmt.Printf("diagnostics=%d\n", len(bank.Diagnostics))
	for _, diagnostic := range bank.Diagnostics {
		rel, err := filepath.Rel(bank.Root, diagnostic.Path)
		if err != nil {
			rel = diagnostic.Path
		}
		fmt.Fprintf(os.Stderr, "%s:%d: %s\n", rel, diagnostic.Line, diagnostic.Message)
	}
}
