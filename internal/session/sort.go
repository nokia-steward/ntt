package session

import "sort"

type SessionSlice []session

func (s SessionSlice) Len() int           { return len(s) }
func (s SessionSlice) Less(i, j int) bool { return s[i].num < s[j].num }
func (s SessionSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func Sort(sessions []session) {
	sort.Sort(SessionSlice(sessions))
}
