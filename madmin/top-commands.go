package madmin

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

// LockEntry holds information about client requesting the lock,
// servers holding the lock, source on the client machine,
// ID, type(read or write) and time stamp.
type LockEntry struct {
	Timestamp  time.Time `json:"time"`       // Timestamp set at the time of initialization.
	Resource   string    `json:"resource"`   // Resource contains info like bucket, object etc
	Type       string    `json:"type"`       // Bool whether write or read lock.
	Source     string    `json:"source"`     // Source which created the lock
	ServerList []string  `json:"serverlist"` // RPC path of servers issuing the lock.
	Owner      string    `json:"owner"`      // RPC path of client claiming lock.
	ID         string    `json:"id"`         // UID to uniquely identify request of client.
}

// LockEntries - To sort the locks
type LockEntries []LockEntry

func (l LockEntries) Len() int {
	return len(l)
}

func (l LockEntries) Less(i, j int) bool {
	return l[i].Timestamp.Before(l[j].Timestamp)
}

func (l LockEntries) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

// TopLocks - returns the oldest locks in a minio setup.
func (adm *AdminClient) TopLocks() (LockEntries, error) {
	// Execute GET on /minio/admin/v2/top/locks
	// to get the oldest locks in a minio setup.
	resp, err := adm.executeMethod("GET",
		requestData{relPath: adminAPIPrefix + "/top/locks"})
	defer closeResponse(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, httpRespToErrorResponse(resp)
	}

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return LockEntries{}, err
	}

	var lockEntries LockEntries
	err = json.Unmarshal(response, &lockEntries)
	return lockEntries, err
}
