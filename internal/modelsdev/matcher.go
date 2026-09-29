package modelsdev

import (
	"regexp"
	"sort"
	"strings"
)

// Matcher performs intelligent lookup of model names against a Catalog.
type Matcher struct {
	catalog Catalog
	// Precomputed index: normalized model key -> ModelData
	flatModels map[string]ModelData
}

// NewMatcher creates a Matcher from a Catalog and optional official providers mapping.
func NewMatcher(catalog Catalog, officialProviders ...map[string][]string) *Matcher {
	flat := make(map[string]ModelData)
	providerIDs := make([]string, 0, len(catalog))
	for providerID := range catalog {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Strings(providerIDs)

	var authMap map[string][]string
	if len(officialProviders) > 0 && officialProviders[0] != nil {
		authMap = officialProviders[0]
	}

	// Precompile regex patterns for official providers
	compiledOfficial := make(map[string][]*regexp.Regexp)
	if len(authMap) > 0 {
		for pID, patterns := range authMap {
			for _, pat := range patterns {
				pat = strings.TrimSpace(pat)
				if pat == "" {
					continue
				}
				re, err := regexp.Compile(pat)
				if err == nil {
					compiledOfficial[pID] = append(compiledOfficial[pID], re)
				}
			}
		}
	}

	// Pass 1: Index official models first so official vendor definitions take priority.
	if len(compiledOfficial) > 0 {
		for _, providerID := range providerIDs {
			provider := catalog[providerID]
			modelIDs := make([]string, 0, len(provider.Models))
			for id := range provider.Models {
				modelIDs = append(modelIDs, id)
			}
			sort.Strings(modelIDs)
			for _, id := range modelIDs {
				if isOfficial(compiledOfficial, providerID, id) {
					indexModel(flat, providerID, id, provider.Models[id])
				}
			}
		}
	}

	// Pass 2: Index remaining/fallback models in deterministic provider order.
	for _, providerID := range providerIDs {
		provider := catalog[providerID]
		modelIDs := make([]string, 0, len(provider.Models))
		for id := range provider.Models {
			modelIDs = append(modelIDs, id)
		}
		sort.Strings(modelIDs)
		for _, id := range modelIDs {
			indexModel(flat, providerID, id, provider.Models[id])
		}
	}

	return &Matcher{
		catalog:    catalog,
		flatModels: flat,
	}
}

func indexModel(flat map[string]ModelData, providerID, id string, m ModelData) {
	m.ProviderID = providerID
	addModelIndex(flat, id, m)
	addModelIndex(flat, normalizeKey(id), m)

	// Index with explicit provider prefix if not already present (e.g. siliconflow/deepseek-ai/DeepSeek-V4-Flash)
	if !strings.HasPrefix(id, providerID+"/") {
		fullID := providerID + "/" + id
		addModelIndex(flat, fullID, m)
		addModelIndex(flat, normalizeKey(fullID), m)
	}

	// Index without provider prefix (e.g., anthropic/claude-3-7 -> claude-3-7)
	if slashIdx := strings.LastIndex(id, "/"); slashIdx != -1 {
		shortID := id[slashIdx+1:]
		addModelIndex(flat, shortID, m)
		addModelIndex(flat, normalizeKey(shortID), m)
	}
}

func isOfficial(compiled map[string][]*regexp.Regexp, providerID, modelID string) bool {
	if len(compiled) == 0 {
		return false
	}
	regexes, ok := compiled[providerID]
	if !ok {
		return false
	}
	if len(regexes) == 0 {
		return true
	}
	shortID := modelID
	if slashIdx := strings.LastIndex(modelID, "/"); slashIdx != -1 {
		shortID = modelID[slashIdx+1:]
	}
	normID := normalizeKey(modelID)
	normShortID := normalizeKey(shortID)

	for _, re := range regexes {
		if re.MatchString(modelID) || re.MatchString(shortID) || re.MatchString(normID) || re.MatchString(normShortID) {
			return true
		}
	}
	return false
}

// FindModel searches for a model by id, attempting exact, stripped-prefix, and normalized matches.
func (m *Matcher) FindModel(query string) (*ModelData, bool) {
	if query == "" {
		return nil, false
	}

	// 1. Direct match
	if model, ok := m.flatModels[query]; ok {
		return &model, true
	}

	// 2. Normalized query match
	norm := normalizeKey(query)
	if model, ok := m.flatModels[norm]; ok {
		return &model, true
	}

	// 3. Strip any slash prefix from query
	if slashIdx := strings.LastIndex(query, "/"); slashIdx != -1 {
		shortQuery := query[slashIdx+1:]
		if model, ok := m.flatModels[shortQuery]; ok {
			return &model, true
		}
		if model, ok := m.flatModels[normalizeKey(shortQuery)]; ok {
			return &model, true
		}
	}

	// 4. Date/version snapshot strip (e.g. claude-3-5-sonnet-20241022 -> claude-3-5-sonnet)
	strippedDate := dateRegex.ReplaceAllString(query, "")
	if strippedDate != query {
		if model, ok := m.flatModels[normalizeKey(strippedDate)]; ok {
			return &model, true
		}
	}

	// 5. Prefix search in flatModels (e.g. user queried claude-3-5-sonnet, models has claude-3-5-sonnet-20241022)
	keys := sortedModelKeys(m.flatModels)
	for _, key := range keys {
		model := m.flatModels[key]
		if strings.HasPrefix(key, norm) || strings.HasPrefix(norm, key) {
			return &model, true
		}
	}

	return nil, false
}

var dateRegex = regexp.MustCompile(`[-_](20\d{2}[-_]?\d{2}[-_]?\d{2}|latest)$`)

func normalizeKey(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	return strings.TrimSpace(s)
}

// SuggestCandidates returns close matches for a model query when no exact match was found.
func (m *Matcher) SuggestCandidates(query string, maxResults int) []ModelData {
	norm := normalizeKey(query)
	var candidates []ModelData
	seen := make(map[string]bool)

	for _, key := range sortedModelKeys(m.flatModels) {
		model := m.flatModels[key]
		if seen[model.ID] {
			continue
		}
		if strings.Contains(key, norm) || strings.Contains(norm, key) {
			candidates = append(candidates, model)
			seen[model.ID] = true
			if len(candidates) >= maxResults {
				break
			}
		}
	}
	return candidates
}

func addModelIndex(index map[string]ModelData, key string, model ModelData) {
	if _, exists := index[key]; !exists {
		index[key] = model
	}
}

func sortedModelKeys(index map[string]ModelData) []string {
	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
