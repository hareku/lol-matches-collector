package lolmatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

type Collector struct {
	HttpCli        *http.Client
	OutputDir      string
	MatchStartTime time.Time
}

func (c *Collector) Run(ctx context.Context) error {
	page := 1
	for {
		if err := c.runEntry(ctx, page); err != nil {
			if errors.Is(err, errNoMoreItems) {
				return nil
			}
			return fmt.Errorf("failed run entry page %d: %w", page, err)
		}
		page++
	}
}

var errNoMoreItems = errors.New("no more items")

func (c *Collector) runEntry(ctx context.Context, page int) error {
	entries, err := c.listEntries(ctx, page)
	if err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}
	log.Printf("Found %d entries on page %d", len(entries), page)
	if len(entries) == 0 {
		return errNoMoreItems
	}

	for _, e := range entries {
		s, err := c.getSummoner(ctx, e.SummonerId)
		if err != nil {
			return fmt.Errorf("failed to get summoner: %w", err)
		}
		mids, err := c.listMatchIDs(ctx, s)
		if err != nil {
			return fmt.Errorf("failed to list match ids: %w", err)
		}
		for _, mid := range mids {
			exist, err := c.matchExist(ctx, mid)
			if err != nil {
				return fmt.Errorf("failed to check whether a match exists: %w", err)
			}
			if exist {
				continue
			}

			m, err := c.getMatch(ctx, mid)
			if err != nil {
				return fmt.Errorf("failed to get match: %w", err)
			}

			if err := c.saveMatch(ctx, mid, m); err != nil {
				return fmt.Errorf("failed to save match: %w", err)
			}
		}
	}
	return nil
}

type entry struct {
	LeagueId   string
	SummonerId string
}

func (c *Collector) listEntries(ctx context.Context, page int) ([]*entry, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://jp1.api.riotgames.com/lol/league/v4/entries/RANKED_SOLO_5x5/SILVER/I?page=%d", page),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	resp, err := c.HttpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		dump, _ := httputil.DumpResponse(resp, true)
		return nil, fmt.Errorf("got error status code: %s", dump)
	}

	entries := []*entry{}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	return entries, nil
}

type summoner struct {
	PUUID string `json:"puuid"`
}

func (c *Collector) getSummoner(ctx context.Context, summonerId string) (*summoner, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://jp1.api.riotgames.com/lol/summoner/v4/summoners/%s", summonerId),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	resp, err := c.HttpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		dump, _ := httputil.DumpResponse(resp, true)
		return nil, fmt.Errorf("got error status code: %s", dump)
	}

	s := &summoner{}
	if err := json.NewDecoder(resp.Body).Decode(s); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	return s, nil
}

type matchID string

func (c *Collector) listMatchIDs(ctx context.Context, s *summoner) ([]matchID, error) {
	q := url.Values{
		"type":      []string{"ranked"},
		"count":     []string{"10"},
		"startTime": []string{fmt.Sprintf("%d", c.MatchStartTime.Unix())},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://asia.api.riotgames.com/lol/match/v5/matches/by-puuid/%s/ids?%s", s.PUUID, q.Encode()),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	resp, err := c.HttpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		dump, _ := httputil.DumpResponse(resp, true)
		return nil, fmt.Errorf("got error status code: %s", dump)
	}

	res := []matchID{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}

	return res, nil
}

type match []byte

func (c *Collector) getMatch(ctx context.Context, mid matchID) (match, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("https://asia.api.riotgames.com/lol/match/v5/matches/%s", mid),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	resp, err := c.HttpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		dump, _ := httputil.DumpResponse(resp, true)
		return nil, fmt.Errorf("got error status code: %s", dump)
	}

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	return res, nil
}

func (c *Collector) matchExist(ctx context.Context, mid matchID) (bool, error) {
	_, err := os.Stat(filepath.Join(c.OutputDir, fmt.Sprintf("%s.json", mid)))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("os.Stat: %w", err)
	}

	return true, nil
}

func (c *Collector) saveMatch(ctx context.Context, mid matchID, m match) error {
	f, err := os.Create(filepath.Join(c.OutputDir, fmt.Sprintf("%s.json", mid)))
	if err != nil {
		return fmt.Errorf("os.Create: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(m); err != nil {
		return fmt.Errorf("writing failed: %w", err)
	}

	return nil
}
