package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/vieGPT/serpapi-trends/internal/client"
	"github.com/vieGPT/serpapi-trends/internal/logic"
	"github.com/vieGPT/serpapi-trends/internal/store"
	"github.com/spf13/cobra"
)

var (
	flagJSON    bool
	flagCompact bool
	flagNoCache bool
	flagHL      string
)

func main() {
	root := &cobra.Command{
		Use:   "serpapi-trends",
		Short: "SerpAPI Google Trends CLI — interest, trending now, autocomplete + local portfolio",
		Long: `serpapi-trends turns SerpAPI's three Google Trends engines into a local data portfolio member.
Detect rising niches (volume + slope + related) before they saturate. Auth via SERPAPI_API_KEY only.`,
		SilenceUsage: true,
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "output full JSON")
	root.PersistentFlags().BoolVar(&flagCompact, "compact", false, "compact human output")
	root.PersistentFlags().BoolVar(&flagNoCache, "no-cache", false, "force fresh SerpAPI fetch (costs quota)")
	root.PersistentFlags().StringVar(&flagHL, "hl", "", "language code (e.g. en)")

	root.AddCommand(newDoctorCmd())
	root.AddCommand(newAccountCmd())
	root.AddCommand(newAutocompleteCmd())
	root.AddCommand(newTrendingCmd())
	root.AddCommand(newInterestCmd())
	root.AddCommand(newRelatedCmd())
	root.AddCommand(newRegionCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newStaleCmd())
	root.AddCommand(newOpportunitiesCmd())
	root.AddCommand(newChangesCmd())
	root.AddCommand(newGeoGapCmd())
	root.AddCommand(newSeasonalityCmd())

	if err := root.Execute(); err != nil {
		msg := err.Error()
		switch {
		case contains(msg, "auth:"):
			os.Exit(2)
		case contains(msg, "rate-limit:"):
			os.Exit(3)
		case contains(msg, "network:"):
			os.Exit(4)
		case contains(msg, "input:"):
			os.Exit(1)
		case contains(msg, "api-error:"):
			os.Exit(5)
		default:
			os.Exit(1)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (stringIndex(s, sub) >= 0)))
}
func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func mustClient() *client.Client {
	c, err := client.NewFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return c
}

func saveSnapshot(engine string, params map[string]string, response map[string]interface{}) {
	s, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: open failed: %v\n", err)
		return
	}
	defer s.Close()
	if err := s.Save(engine, params, response); err != nil {
		fmt.Fprintf(os.Stderr, "store: save failed: %v\n", err)
		return
	}
}

func printResult(v interface{}) {
	if flagJSON || flagCompact {
		enc := json.NewEncoder(os.Stdout)
		if !flagCompact {
			enc.SetIndent("", "  ")
		}
		_ = enc.Encode(v)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check key presence (fingerprint only) and live reachability",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := client.NewFromEnv()
			if err != nil {
				fmt.Println("key: missing")
				fmt.Println("status: blocked")
				return err
			}
			fmt.Printf("key: present (fingerprint %s)\n", c.KeyFingerprint())
			res, err := c.Autocomplete("test", "", true)
			if err != nil {
				fmt.Printf("reachability: FAIL (%v)\n", err)
				return err
			}
			meta, _ := res["search_metadata"].(map[string]interface{})
			status, _ := meta["status"].(string)
			id, _ := meta["id"].(string)
			fmt.Printf("reachability: OK (status=%s id=%s)\n", status, id)
			if _, ok := res["suggestions"]; ok {
				fmt.Println("suggestions: present")
			}
			return nil
		},
	}
}

func newAccountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Show plan / remaining searches (free Account API)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := mustClient()
			res, err := c.Account()
			if err != nil {
				return err
			}
			printResult(res)
			return nil
		},
	}
}

func newAutocompleteCmd() *cobra.Command {
	var q string
	cmd := &cobra.Command{
		Use:     "autocomplete",
		Aliases: []string{"suggest"},
		Short:   "Google Trends autocomplete / topic ID resolution",
		RunE: func(cmd *cobra.Command, args []string) error {
			if q == "" && len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q is required")
			}
			c := mustClient()
			res, err := c.Autocomplete(q, flagHL, flagNoCache)
			if err != nil {
				return err
			}
			params := map[string]string{"q": q}
			if flagHL != "" {
				params["hl"] = flagHL
			}
			saveSnapshot("google_trends_autocomplete", params, res)
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&q, "q", "q", "", "query prefix")
	return cmd
}

func newTrendingCmd() *cobra.Command {
	var geo, hours, categoryID string
	var onlyActive bool
	cmd := &cobra.Command{
		Use:     "trending",
		Aliases: []string{"now"},
		Short:   "Trending Now (volume, increase %, active window)",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := mustClient()
			res, err := c.TrendingNow(geo, hours, categoryID, onlyActive, flagNoCache)
			if err != nil {
				return err
			}
			params := map[string]string{"geo": geo, "hours": hours}
			if categoryID != "" {
				params["category_id"] = categoryID
			}
			if onlyActive {
				params["only_active"] = "true"
			}
			saveSnapshot("google_trends_trending_now", params, res)
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&geo, "geo", "US", "geo (default US)")
	cmd.Flags().StringVar(&hours, "hours", "24", "4|24|48|168")
	cmd.Flags().StringVar(&categoryID, "category-id", "", "category id")
	cmd.Flags().BoolVar(&onlyActive, "only-active", false, "only currently active trends")
	return cmd
}

func newInterestCmd() *cobra.Command {
	var q, dataType, geo, date, gprop, cat, tz string
	cmd := &cobra.Command{
		Use:   "interest",
		Short: "Interest over time (TIMESERIES) or other data_type",
		RunE: func(cmd *cobra.Command, args []string) error {
			if q == "" && len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q is required")
			}
			if dataType == "" {
				dataType = "TIMESERIES"
			}
			c := mustClient()
			res, err := c.Trends(q, dataType, geo, date, "", gprop, cat, tz, flagHL, false, flagNoCache)
			if err != nil {
				return err
			}
			params := map[string]string{"q": q, "data_type": dataType}
			if geo != "" {
				params["geo"] = geo
			}
			if date != "" {
				params["date"] = date
			}
			if gprop != "" {
				params["gprop"] = gprop
			}
			if cat != "" {
				params["cat"] = cat
			}
			if tz != "" {
				params["tz"] = tz
			}
			if flagHL != "" {
				params["hl"] = flagHL
			}
			saveSnapshot("google_trends", params, res)
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&q, "q", "q", "", "query or comma-separated (max 5 for TIMESERIES)")
	cmd.Flags().StringVar(&dataType, "data-type", "TIMESERIES", "TIMESERIES|GEO_MAP|GEO_MAP_0|RELATED_TOPICS|RELATED_QUERIES")
	cmd.Flags().StringVar(&geo, "geo", "", "geo (empty = Worldwide)")
	cmd.Flags().StringVar(&date, "date", "", "date range preset or custom")
	cmd.Flags().StringVar(&gprop, "gprop", "", "images|news|froogle|youtube")
	cmd.Flags().StringVar(&cat, "cat", "", "category id")
	cmd.Flags().StringVar(&tz, "tz", "", "timezone offset minutes")
	return cmd
}

func newRelatedCmd() *cobra.Command {
	var q, dataType, geo, date string
	cmd := &cobra.Command{
		Use:   "related",
		Short: "Related topics or related queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if q == "" && len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q is required")
			}
			if dataType == "" {
				dataType = "RELATED_QUERIES"
			}
			c := mustClient()
			res, err := c.Trends(q, dataType, geo, date, "", "", "", "", flagHL, false, flagNoCache)
			if err != nil {
				return err
			}
			params := map[string]string{"q": q, "data_type": dataType}
			if geo != "" {
				params["geo"] = geo
			}
			if date != "" {
				params["date"] = date
			}
			if flagHL != "" {
				params["hl"] = flagHL
			}
			saveSnapshot("google_trends", params, res)
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&q, "q", "q", "", "single query")
	cmd.Flags().StringVar(&dataType, "data-type", "RELATED_QUERIES", "RELATED_QUERIES|RELATED_TOPICS")
	cmd.Flags().StringVar(&geo, "geo", "", "geo")
	cmd.Flags().StringVar(&date, "date", "", "date range")
	return cmd
}

func newRegionCmd() *cobra.Command {
	var q, dataType, geo, region, date string
	var includeLow bool
	cmd := &cobra.Command{
		Use:   "region",
		Short: "Interest by region or compared breakdown by region",
		RunE: func(cmd *cobra.Command, args []string) error {
			if q == "" && len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q is required")
			}
			if dataType == "" {
				if contains(q, ",") {
					dataType = "GEO_MAP"
				} else {
					dataType = "GEO_MAP_0"
				}
			}
			c := mustClient()
			res, err := c.Trends(q, dataType, geo, date, region, "", "", "", flagHL, includeLow, flagNoCache)
			if err != nil {
				return err
			}
			params := map[string]string{"q": q, "data_type": dataType}
			if geo != "" {
				params["geo"] = geo
			}
			if date != "" {
				params["date"] = date
			}
			if region != "" {
				params["region"] = region
			}
			if includeLow {
				params["include_low_search_volume"] = "true"
			}
			if flagHL != "" {
				params["hl"] = flagHL
			}
			saveSnapshot("google_trends", params, res)
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&q, "q", "q", "", "query (or multi for GEO_MAP)")
	cmd.Flags().StringVar(&dataType, "data-type", "", "GEO_MAP|GEO_MAP_0 (auto if empty)")
	cmd.Flags().StringVar(&geo, "geo", "", "geo")
	cmd.Flags().StringVar(&region, "region", "", "COUNTRY|REGION|DMA|CITY")
	cmd.Flags().StringVar(&date, "date", "", "date range")
	cmd.Flags().BoolVar(&includeLow, "include-low-search-volume", false, "include low volume regions")
	return cmd
}

func openStore() (*store.Store, error) {
	return store.Open(store.DefaultPath())
}

func newHistoryCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Local portfolio history (FTS over snapshots)",
	}
	search := &cobra.Command{
		Use:   "search [term]",
		Short: "FTS search over stored snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			term := ""
			if len(args) > 0 {
				term = args[0]
			}
			if term == "" {
				return fmt.Errorf("input: search term required")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			hits, err := logic.SearchHistory(s, term, limit)
			if err != nil {
				return err
			}
			printResult(hits)
			return nil
		},
	}
	search.Flags().IntVar(&limit, "limit", 20, "max hits")
	cmd.AddCommand(search)
	return cmd
}

func newStaleCmd() *cobra.Command {
	var olderThan string
	cmd := &cobra.Command{
		Use:   "stale",
		Short: "List local snapshots older than threshold (pure local)",
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := time.ParseDuration(olderThan)
			if err != nil {
				return fmt.Errorf("input: invalid --older-than duration: %w", err)
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			res, err := logic.FindStale(s, d)
			if err != nil {
				return err
			}
			printResult(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&olderThan, "older-than", "168h", "duration (e.g. 24h, 168h)")
	return cmd
}

func newOpportunitiesCmd() *cobra.Command {
	var limit int
	var source string
	cmd := &cobra.Command{
		Use:   "opportunities [seed]",
		Short: "Rank rising related (default) or trending terms for a seed — single source, local portfolio",
		Long: `Score is source-specific and never mixed:
  --source related  (default): Google related_queries.rising extracted_value (% increase / breakout scale)
  --source trending: trending_searches increase_percentage
  --source all: related block then trending block; scores not cross-comparable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			seed := ""
			if len(args) > 0 {
				seed = args[0]
			}
			if seed == "" {
				return fmt.Errorf("input: seed required")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			out, err := logic.Opportunities(s, seed, limit, source)
			if err != nil {
				return err
			}
			printResult(out)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	cmd.Flags().StringVar(&source, "source", "related", "related|trending|all")
	return cmd
}

func newChangesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changes [q]",
		Short: "Diff related queries across local snapshots for a parent q",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q required")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			out, err := logic.Changes(s, q)
			if err != nil {
				return err
			}
			printResult(out)
			return nil
		},
	}
	return cmd
}

func newGeoGapCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "geo-gap [seed]",
		Short: "Regions of high relative interest from stored GEO snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			seed := ""
			if len(args) > 0 {
				seed = args[0]
			}
			if seed == "" {
				return fmt.Errorf("input: seed required")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			out, err := logic.GeoGap(s, seed, limit)
			if err != nil {
				return err
			}
			printResult(out)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	return cmd
}

func newSeasonalityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seasonality [q]",
		Short: "Monthly averages from stored TIMESERIES snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			if q == "" {
				return fmt.Errorf("input: q required")
			}
			s, err := openStore()
			if err != nil {
				return err
			}
			defer s.Close()
			out, err := logic.Seasonality(s, q)
			if err != nil {
				return err
			}
			printResult(out)
			return nil
		},
	}
	return cmd
}
