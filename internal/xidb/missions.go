package xidb

import "encoding/binary"

// Mission progress, decoded from the chars.missions blob.
//
// The blob is a raw dump of missionlog_t[MAX_MISSIONAREA] from common/mmo.h:
//
//	struct missionlog_t {
//	    uint16 current;
//	    uint16 statusUpper;
//	    uint16 statusLower;
//	    bool   complete[64];
//	}
//
// 6 bytes then 64, no trailing padding because the struct's alignment is 2 and
// 70 is even. Fifteen logs, so 1050 bytes. Little-endian, since the server
// memcpys the struct straight out.
//
// Two rules here are not guessable from the data and come from the server's own
// accessors in lua_baseentity.cpp. Both are load-bearing:
//
//  1. "No current mission" is spelled differently per log. setMissionStatus
//     clears it with `logId > 2 ? 0 : uint16 max`, so the three nation logs use
//     65535 while every expansion log uses 0. Reading 0 as "no mission" for a
//     nation log would hide its first mission; reading it as a mission for an
//     expansion log would invent one for every fresh character.
//
//  2. Completion is a hybrid. hasCompletedMission uses
//     `(log == CoP || id >= 64) ? id < current : complete[id]`, because the
//     complete array only has 64 slots while several logs have more missions
//     than that. Those logs treat `current` as a high-water mark instead.
const (
	missionLogStride   = 70
	missionCompleteCap = 64
	missionNoneNation  = 0xFFFF
	missionLogCoP      = 6
	missionLastNation  = 2
)

// MissionLogDef identifies one mission log. Generated from the server's
// xi.mission.log_id enum, see missions_gen.go.
type MissionLogDef struct {
	LogID int
	Slug  string
	Short string
	Label string
}

// MissionProgress is one log's state for a character.
type MissionProgress struct {
	Log        MissionLogDef
	CurrentID  int
	Current    string
	HasCurrent bool

	// LastDone is the highest-numbered mission the character has finished,
	// which is what "where are they up to" actually means to a reader.
	LastDoneID  int
	LastDone    string
	HasLastDone bool

	Completed int
	Total     int
}

// Percent drives the progress bar. Logs with no mission table report zero.
func (m MissionProgress) Percent() float64 {
	if m.Total <= 0 {
		return 0
	}
	if m.Completed >= m.Total {
		return 100
	}
	return float64(m.Completed) / float64(m.Total) * 100.0
}

func (m MissionProgress) Finished() bool { return m.Total > 0 && m.Completed >= m.Total }

// Started reports whether there is anything worth showing for this log.
func (m MissionProgress) Started() bool { return m.HasCurrent || m.Completed > 0 }

// missionNoCurrent applies rule 1 above.
func missionNoCurrent(logID, current int) bool {
	if logID <= missionLastNation {
		return current == missionNoneNation
	}
	return current == 0
}

// missionDone applies rule 2 above.
func missionDone(logID, missionID, current int, complete []bool) bool {
	if logID == missionLogCoP || missionID >= missionCompleteCap {
		return missionID < current
	}
	if missionID < len(complete) {
		return complete[missionID]
	}
	return false
}

// MissionName returns the display name for a mission, or an empty string when
// the log has no table or the id is unknown.
func MissionName(logID, missionID int) string {
	if byLog, ok := missionNames[logID]; ok {
		return byLog[missionID]
	}
	return ""
}

// decodeMissions reads the blob into one entry per log. A short or absent blob
// yields no entries rather than an error: a character who has never been saved
// by a current server should still render.
func decodeMissions(blob []byte) []MissionProgress {
	if len(blob) < missionLogStride {
		return nil
	}

	out := make([]MissionProgress, 0, len(MissionLogs))

	for _, def := range MissionLogs {
		offset := def.LogID * missionLogStride
		if offset+missionLogStride > len(blob) {
			break
		}

		record := blob[offset : offset+missionLogStride]
		current := int(binary.LittleEndian.Uint16(record[0:2]))

		complete := make([]bool, missionCompleteCap)
		for i := 0; i < missionCompleteCap; i++ {
			complete[i] = record[6+i] != 0
		}

		p := MissionProgress{Log: def, CurrentID: current}

		if !missionNoCurrent(def.LogID, current) {
			p.HasCurrent = true
			p.Current = MissionName(def.LogID, current)
		}

		// Counting over the known mission ids rather than raw bits gives a
		// "12 of 24" that means something, and skips the padding slots.
		for missionID := range missionNames[def.LogID] {
			p.Total++
			if missionDone(def.LogID, missionID, current, complete) {
				p.Completed++
				if !p.HasLastDone || missionID > p.LastDoneID {
					p.HasLastDone = true
					p.LastDoneID = missionID
				}
			}
		}

		if p.HasLastDone {
			p.LastDone = MissionName(def.LogID, p.LastDoneID)
		}

		out = append(out, p)
	}

	return out
}
