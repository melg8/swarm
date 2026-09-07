package main

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/pathfind"
)

func main() {
	engine := pathfind.NewEngine(
		"../l2j_mobius/L2J_Mobius_C1_HarbingersOfWar/dist/game/data/geodata")
	engine.SetMaxPassableHeight(pathfind.DefaultMaxPassableHeight)
	stats := engine.Stats()
	fmt.Printf("geodata: regions=%d hasData=%v dir=%s\n",
		stats.RegionFiles, stats.HasData, stats.Dir)

	// From the hunting zone center to the guard Kendell (47595, 51569, -2992).
	from := pathfind.Vec3{X: 46112, Y: 41500, Z: -3500}
	to := pathfind.Vec3{X: 47595, Y: 51569, Z: -2992}
	res, err := engine.FindPathTo(
		from, to, int16(to.Z), engine.MaxPassableHeight())
	if err != nil {
		fmt.Println("path error:", err)

		return
	}
	if res == nil || !res.Found {
		fmt.Println("path NOT found")

		return
	}
	fmt.Printf("path found: %d waypoints, len %.0f\n",
		len(res.Waypoints), res.Length)
	for i, wp := range res.Waypoints {
		if i < 5 || i > len(res.Waypoints)-3 {
			fmt.Printf("  wp[%d] %.0f %.0f %.0f\n", i, wp.X, wp.Y, wp.Z)
		}
	}
}
