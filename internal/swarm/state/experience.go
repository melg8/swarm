// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

// experienceTable is the cumulative experience needed to reach each
// level of the L2J Mobius C1 data pack
// (dist/game/data/stats/players/experience.xml, table maxLevel 78).
// experienceTable[N-1] is the total experience a character has exactly
// when reaching level N, so the progress toward the next level is
// (exp - experienceTable[level-1]) / (experienceTable[level] -
// experienceTable[level-1]). Values above level 78 exceed the int32
// protocol field and wrap, the percentage is clamped in that case.
// Regenerate with tools/generate_experience_table.sh.
var experienceTable = [...]int64{
	           0,	          68,	         363,	        1168,
	        2884,	        6038,	       11287,	       19423,
	       31378,	       48229,	       71201,	      101676,
	      141192,	      191452,	      254327,	      331864,
	      426284,	      539995,	      675590,	      835854,
	     1023775,	     1242536,	     1495531,	     1786365,
	     2118860,	     2497059,	     2925229,	     3407873,
	     3949727,	     4555766,	     5231213,	     5981539,
	     6812472,	     7729999,	     8740372,	     9850111,
	    11066012,	    12395149,	    13844879,	    15422851,
	    17137002,	    18995573,	    21007103,	    23180442,
	    25524751,	    28049509,	    30764519,	    33679907,
	    36806133,	    40153995,	    45524865,	    51262204,
	    57383682,	    63907585,	    70852742,	    80700339,
	    91162131,	   102265326,	   114038008,	   126509030,
	   146307211,	   167243291,	   189363788,	   212716741,
	   237351413,	   271973532,	   308441375,	   346825235,
	   387197529,	   429632402,	   474205751,	   532692055,
	   606319094,	   696376867,	   804219972,	   931275828,
	  1151275834,	  1511275834,	  2099275834,	  4200000000,
	  6300000000,
}

// maxExperienceLevel is the highest level the table knows about.
const maxExperienceLevel = 81

// ExpPercent returns the experience progress of a character toward the
// next level as a percentage (0..100) based on the C1 experience table.
// Unknown levels and wrapped values clamp to the nearest bound.
func ExpPercent(level int32, exp int64) float64 {
	if level < 1 {
		return 0
	}
	if level > maxExperienceLevel {
		return 100
	}
	base := experienceTable[level-1]
	next := experienceTable[level]
	if next <= base || exp <= base {
		return 0
	}
	if exp >= next {
		return 100
	}

	return float64(exp-base) / float64(next-base) * 100
}
