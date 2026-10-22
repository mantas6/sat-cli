package api

import (
	"encoding/json"
	"time"
)

// Article is an article summary returned by the journal API.
type Article struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	WordCount int      `json:"word_count"`
	CreatedAt string   `json:"created_at"`
	Journal   *Journal `json:"journal"`
}

// Journal identifies an article journal.
type Journal struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// ArticleContents is the full Markdown response for an article.
type ArticleContents struct {
	Contents string `json:"contents"`
}

type articleContentsRequest struct {
	Contents string `json:"contents"`
}

type articleJournalRequest struct {
	Journal string `json:"journal"`
}

// savedTrack is a single saved-track line from the saved albums endpoint.
type savedTrack struct {
	Line string `json:"line"`
}

// Machine is a machine's latest report returned by the machines API. Numbers
// are nil and strings are empty when the module was not reported; a null
// nested object decodes to its zero value.
type Machine struct {
	Name          string             `json:"name"`
	Online        bool               `json:"online"`
	ReceivedAt    *time.Time         `json:"received_at"`
	Uptime        MachineUptime      `json:"uptime"`
	KernelVersion string             `json:"kernel_version"`
	CPU           MachineCPU         `json:"cpu"`
	LoadAverage   MachineLoadAverage `json:"load_average"`
	Memory        MachineMemory      `json:"memory"`
	Disk          MachineDisk        `json:"disk"`
	Stats         []MachineStat      `json:"stats,omitempty"` // GetMachine only
	Raw           json.RawMessage    `json:"-"`               // object as sent; set by UnmarshalJSON
}

// MachineUptime is how long a machine has been running.
type MachineUptime struct {
	Milliseconds *int64 `json:"milliseconds"`
	ForHumans    string `json:"for_humans"`
	ExceedsDay   bool   `json:"exceeds_day"`
}

// MachineCPU describes a machine's processor.
type MachineCPU struct {
	Name            string   `json:"name"`
	Temperature     *float64 `json:"temperature"`
	UsagePercentage *float64 `json:"usage_percentage"`
}

// MachineLoadAverage is a machine's 1, 5 and 15 minute load average.
type MachineLoadAverage struct {
	One     *float64 `json:"1m"`
	Five    *float64 `json:"5m"`
	Fifteen *float64 `json:"15m"`
}

// MachineMemory describes a machine's memory and swap usage in gigabytes.
type MachineMemory struct {
	UsagePercentage         *float64 `json:"usage_percentage"`
	UsageWithSwapPercentage *float64 `json:"usage_with_swap_percentage"`
	UsageHigh               bool     `json:"usage_high"`
	UsedGB                  *float64 `json:"used_gb"`
	UsedWithSwapGB          *float64 `json:"used_with_swap_gb"`
	TotalGB                 *float64 `json:"total_gb"`
	SwapUsedGB              *float64 `json:"swap_used_gb"`
	SwapTotalGB             *float64 `json:"swap_total_gb"`
}

// MachineDisk describes a machine's disk usage in gigabytes.
type MachineDisk struct {
	UsagePercentage *float64 `json:"usage_percentage"`
	FreePercentage  *float64 `json:"free_percentage"`
	UsageHigh       bool     `json:"usage_high"`
	UsedGB          *float64 `json:"used_gb"`
	FreeGB          *float64 `json:"free_gb"`
	TotalGB         *float64 `json:"total_gb"`
	Temperature     *float64 `json:"temperature"`
}

// MachineStat is one stats module of a machine's report. Result is kept as
// sent because its shape depends on Type.
type MachineStat struct {
	Type   string          `json:"type"`
	Result json.RawMessage `json:"result"`
}

// UnmarshalJSON decodes a machine and keeps a copy of data in Raw. A JSON null
// leaves the machine unchanged.
func (m *Machine) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	// machine has Machine's fields but not this method, so decoding into it
	// does not recurse.
	type machine Machine
	var decoded machine
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Machine(decoded)
	// The decoder may reuse data's buffer after this call returns.
	m.Raw = append(json.RawMessage(nil), data...)

	return nil
}
