// 作成日: 2026-10-25。教材紹介だけを保存する。戦績・棋力の再計算と描画内のファイル書込みは禁止する。
package app

import (
	"errors"
	"math"
	"slices"
)

// validateTechniques は省略・nullを未紹介とし、未知ID・重複・不正な紹介順を拒否する。
func (p Profile) validateTechniques() error {
	if len(p.Techniques) > len(TechniqueIDs()) || p.Version == 1 && (len(p.Techniques) > 0 || p.LastTechniqueGame != "") {
		return errors.New("invalid technique history version or size")
	}
	if len(p.Techniques) == 0 {
		if p.LastTechniqueGame != "" {
			return errors.New("technique game without history")
		}
		return nil
	}
	if !validID(p.LastTechniqueGame) {
		return errors.New("invalid technique game id")
	}
	ids := make(map[TechniqueID]bool)
	orders := make(map[uint64]bool)
	var total uint64
	for _, r := range p.Techniques {
		total += uint64(r.Count)
	}
	for _, r := range p.Techniques {
		if !slices.Contains(TechniqueIDs(), r.ID) || ids[r.ID] || r.Count == 0 || r.LastShown == 0 || r.LastShown > total || orders[r.LastShown] {
			return errors.New("invalid technique history")
		}
		ids[r.ID], orders[r.LastShown] = true, true
	}
	return nil
}

// ChooseTechnique は準備済みの概要から一局につき一つの教材を固定する。保存は行わない。
func (c *Controller) ChooseTechnique() *TechniqueLesson {
	if c.Technique != nil {
		return c.Technique
	}
	if c.Phase != Summary || c.SummarySession == nil || !c.SummarySession.Ready {
		return nil
	}
	c.Technique = SelectTechnique(c.SummarySession.Lessons, c.Profile.Techniques, c.Player)
	return c.Technique
}

// RecordTechniqueShown は画面所有者が描画後に呼ぶ。失敗時は履歴・Revisionを変えず、同じ教材で再試行できる。
func (c *Controller) RecordTechniqueShown() error {
	if c.Phase != Summary || c.Technique == nil || c.SummarySession == nil || !c.SummarySession.Ready {
		return errors.New("technique has not been presented")
	}
	if c.techniqueRecorded || c.Profile.LastTechniqueGame == c.gameID {
		c.techniqueRecorded = true
		return nil
	}
	if !validID(c.gameID) {
		return errors.New("invalid technique game id")
	}
	p := c.Profile
	p.Version = ProfileVersion
	p.Techniques = slices.Clone(p.Techniques)
	var total uint64
	index := -1
	for i, r := range p.Techniques {
		total += uint64(r.Count)
		if r.ID == c.Technique.ID {
			index = i
		}
	}
	if index < 0 {
		p.Techniques = append(p.Techniques, TechniqueRecord{ID: c.Technique.ID})
		index = len(p.Techniques) - 1
	}
	if p.Techniques[index].Count == math.MaxUint32 {
		return errors.New("technique counter exhausted")
	}
	p.Techniques[index].Count++
	p.Techniques[index].LastShown = total + 1
	p.LastTechniqueGame = c.gameID
	if err := c.store.Save(&p); err != nil {
		return err
	}
	c.Profile = p
	c.techniqueRecorded = true
	return nil
}
