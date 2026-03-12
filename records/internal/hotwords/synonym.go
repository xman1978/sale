package hotwords

// NormalizeTerm 使用同义词词典归一化关键词；若词典中存在则返回 target_term，否则返回原词
func NormalizeTerm(term string, synonyms map[string]string) string {
	if t, ok := synonyms[term]; ok {
		return t
	}
	return term
}
