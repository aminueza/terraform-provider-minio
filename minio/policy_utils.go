package minio

import (
	"sort"

	"github.com/minio/minio-go/v7/pkg/set"
)

// ConditionKeyMap - map of policy condition key and value.
type ConditionKeyMap map[string]set.StringSet

// Add - adds key and value.  The value is appended If key already exists.
func (ckm ConditionKeyMap) Add(key string, value set.StringSet) {
	if v, ok := ckm[key]; ok {
		ckm[key] = v.Union(value)
	} else {
		ckm[key] = set.CopyStringSet(value)
	}
}

// Remove - removes value of given key.  If key has empty after removal, the key is also removed.
func (ckm ConditionKeyMap) Remove(key string, value set.StringSet) {
	if v, ok := ckm[key]; ok {
		if value != nil {
			ckm[key] = v.Difference(value)
		}

		if ckm[key].IsEmpty() {
			delete(ckm, key)
		}
	}
}

// RemoveKey - removes key and its value.
func (ckm ConditionKeyMap) RemoveKey(key string) {
	if _, ok := ckm[key]; ok {
		delete(ckm, key)
	}
}

// CopyConditionKeyMap - returns new copy of given ConditionKeyMap.
func CopyConditionKeyMap(condKeyMap ConditionKeyMap) ConditionKeyMap {
	out := make(ConditionKeyMap)

	for k, v := range condKeyMap {
		out[k] = set.CopyStringSet(v)
	}

	return out
}

// mergeConditionKeyMap - returns a new ConditionKeyMap which contains merged key/value of given two ConditionKeyMap.
func mergeConditionKeyMap(condKeyMap1 ConditionKeyMap, condKeyMap2 ConditionKeyMap) ConditionKeyMap {
	out := CopyConditionKeyMap(condKeyMap1)

	for k, v := range condKeyMap2 {
		if ev, ok := out[k]; ok {
			out[k] = ev.Union(v)
		} else {
			out[k] = set.CopyStringSet(v)
		}
	}

	return out
}

// ConditionMap - map of condition and conditional values.
type ConditionMap map[string]ConditionKeyMap

// Add - adds condition key and condition value.  The value is appended if key already exists.
func (cond ConditionMap) Add(condKey string, condKeyMap ConditionKeyMap) {
	if v, ok := cond[condKey]; ok {
		cond[condKey] = mergeConditionKeyMap(v, condKeyMap)
	} else {
		cond[condKey] = CopyConditionKeyMap(condKeyMap)
	}
}

// Remove - removes condition key and its value.
func (cond ConditionMap) Remove(condKey string) {
	if _, ok := cond[condKey]; ok {
		delete(cond, condKey)
	}
}

// mergeConditionMap - returns new ConditionMap which contains merged key/value of two ConditionMap.
func mergeConditionMap(condMap1 ConditionMap, condMap2 ConditionMap) ConditionMap {
	out := make(ConditionMap)

	for k, v := range condMap1 {
		out[k] = CopyConditionKeyMap(v)
	}

	for k, v := range condMap2 {
		if ev, ok := out[k]; ok {
			out[k] = mergeConditionKeyMap(ev, v)
		} else {
			out[k] = CopyConditionKeyMap(v)
		}
	}

	return out
}

func minioDecodePolicyStringList(lI []interface{}) interface{} {

	if len(lI) == 1 {
		return lI[0].(string)
	}
	ret := make([]string, len(lI))
	for i, vI := range lI {
		ret[i] = vI.(string)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ret)))
	return ret
}

func (s *IAMPolicyDoc) merge(newDoc *IAMPolicyDoc) {
	// adopt newDoc's Id
	if len(newDoc.ID) > 0 {
		s.ID = newDoc.ID
	}

	// let newDoc upgrade our Version
	if newDoc.Version > s.Version {
		s.Version = newDoc.Version
	}

	// merge in newDoc's statements, overwriting any existing Sids
	var seen bool
	for _, newStatement := range newDoc.Statements {
		if len(newStatement.Sid) == 0 {
			s.Statements = append(s.Statements, newStatement)
			continue
		}
		seen = false
		for i, existingStatement := range s.Statements {
			if existingStatement.Sid == newStatement.Sid {
				s.Statements[i] = newStatement
				seen = true
				break
			}
		}
		if !seen {
			s.Statements = append(s.Statements, newStatement)
		}
	}
}
