//go:build !gopacabra || !gopacabrastat

package runtime

// Без тегу gopacabrastat покинуті масиви росту (P-NEW-03.6) не рахуються:
// порожні тіла компілятор вбудовує і прибирає.
func noteGrowSlice(oldCap, etSize int) {}

func noteEvacuate(t *maptype, h *hmap, b *bmap) {}

func noteGrowDone(t *maptype, h *hmap) {}
