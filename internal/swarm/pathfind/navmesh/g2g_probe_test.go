package navmesh

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestGludioToGiranOneShot probes the road leg Gludio -> Giran (the
// land route around the inner bay).
func TestGludioToGiranOneShot(t *testing.T) {
    dir := "../../../../data/navmesh"
    mesh := NewMesh(dir)
    if len(mesh.TileFiles()) < 100 {
        t.Skip("the whole map pack is not present")
    }
    mesh.SetCacheCapacity(4)
    gludio := Pos{X: -12787, Y: 122779, Z: -3114}
    giran := Pos{X: 83336, Y: 147972, Z: -3404}
    startRef, startPos, ok := mesh.FindNearestPoly(gludio)
    require.True(t, ok)
    endRef, endPos, ok := mesh.FindNearestPoly(giran)
    require.True(t, ok)
    sc, sr := TileOf(startRef)
    ec, er := TileOf(endRef)
    t.Logf("tiles %d_%d -> %d_%d, straight %.0f", sc, sr, ec, er,
        dist3(startPos, endPos))
    began := time.Now()
    route, err := mesh.Route(startPos, endPos, DefaultFilter())
    require.NoError(t, err)
    t.Logf("Gludio->Giran: found=%t partial=%t hier=%t explored=%d"+
        " wps=%d length=%.0f %s", route.Found, route.Partial,
        route.Hierarchical, route.Explored, len(route.Waypoints),
        routeLength(route), time.Since(began))
}
