-- npc_firework (cluster fireworks) — Lua port of npc_fireworkAI::Reset
-- from src/server/scripts/World/npcs_special.cpp (:1760-1996, enum
-- Fireworks :1710-1760). The cluster entries are the isCluster()
-- true set (:1778-1808); the single-firework entries are NOT ported
-- here (see the stay-outs below). Eluna creature event: 23 OnReset.
-- Event 23 fires on spawn and on leave-combat in Go (matching the
-- C++ Reset call sites) and also on death, so the handler guards on
-- IsAlive (the prowlerReset precedent): C++ Reset never runs on a
-- corpse, and the firework has no JustDied arm to twin.
--
-- Ported arms, in C++ Reset order (:1938-1982):
-- Launcher (:1939-1946): FindNearestLauncher's priority-ordered
-- GetClosestGameObjectWithEntry(me, entry, 0.5) chain over the four
-- cluster launchers twins as sequential creature:FindNearestGameObject
-- (entry, 0.5) calls, first non-nil wins; nil across all four returns
-- early exactly like the C++ else-return. launcher:SendCustomAnim(3)
-- is the bridged GO anim (ANIM_GO_LAUNCH_FIREWORK, :1763).
-- Orientation (:1943): me->SetOrientation(launcher:GetOrientation()
-- + pi/2) twins as creature:SetFacingTo with the sum normalized to
-- [0, 2pi); SetFacingTo is the in-tree facing twin (Unit::SetFacingTo)
-- and broadcasts the turn the C++ silent slot write does not — the
-- only observable difference, and the facing itself is C++-exact.
-- The go:GetO() read is the new GO-orientation bridge
-- (luaGameObjectState.Orientation, gameobject.orientation column).
-- Moonglade (:1950-1967): the GetZoneId (493) + no-Omen-within-100
-- + within-100-of-omenSummonPos gates twin via creature:GetZoneId,
-- creature:FindNearestCreature(15467, 100.0) and a 2D distance check
-- against omenSummonPos (7558.993, -2839.999); the urand(0, 9) roll
-- twins as math.random(10) - 1, and roll 9 summons Omen at
-- omenSummonPos via creature:SummonCreature with type 7
-- MANUAL_DESPAWN / 0ms (the C++ SummonCreature(entry, pos) default,
-- Object.h:412). Rolls 0-3 (minion of Omen) stay out: the minion's
-- AttackStart(SelectNearestPlayer) leg has no bridge, and summoning
-- an idle minion would be a half-pair state C++ never produces
-- (kodo/balinda precedent) — the roll distribution is preserved by
-- leaving those rolls as no-ops.
-- Elune (:1979-1980): CLUSTER_ELUNE (15918) casts SPELL_LUNAR_FORTUNE
-- 26522 on itself, non-triggered (C++ DoCast default).
--
-- Stay-outs, all riding this file's Reset handler (never a separate
-- arm): the four SummonGameObject legs (:1982) — GetFireworkGameObjectId
-- reads sSpellMgr spell-effect data (SPELL_EFFECT_SUMMON_OBJECT_WILD
-- MiscValue) and there is no Spell.dbc in-tree to verify the summoned
-- GO entries against, so no entry is invented here; the single
-- firework leg (:1984-1985) — me->CastSpell(me->GetPosition(), spell,
-- true) is a position-target triggered cast with no Lua bridge
-- (creature:CastSpell takes a unit target or nil-victim only); the
-- cluster entries' own spell legs are the GO-summon legs above, not
-- self-casts.

local CLUSTER_ENTRIES = {
    15872, 15873, 15874, 15875, 15876, 15877,
    15911, 15912, 15913, 15914, 15915, 15916,
    15918,
}
local CLUSTER_LAUNCHERS = { 180772, 180859, 180869, 180874 }
local ENTRY_CLUSTER_ELUNE = 15918
local ENTRY_OMEN = 15467
local ZONE_MOONGLADE = 493
local SPELL_LUNAR_FORTUNE = 26522
local ANIM_GO_LAUNCH_FIREWORK = 3
local OMEN_X, OMEN_Y, OMEN_Z, OMEN_O = 7558.993, -2839.999, 450.0214, 4.46

local function fireworkReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    local launcher = nil
    for _, entry in ipairs(CLUSTER_LAUNCHERS) do
        launcher = creature:FindNearestGameObject(entry, 0.5)
        if launcher ~= nil then
            break
        end
    end
    if launcher == nil then
        return
    end
    launcher:SendCustomAnim(ANIM_GO_LAUNCH_FIREWORK)
    creature:SetFacingTo((launcher:GetO() + math.pi / 2) % (2 * math.pi))
    if creature:GetZoneId() == ZONE_MOONGLADE then
        if creature:FindNearestCreature(ENTRY_OMEN, 100.0) == nil then
            local dx = creature:GetX() - OMEN_X
            local dy = creature:GetY() - OMEN_Y
            if math.sqrt(dx * dx + dy * dy) <= 100.0 then
                local roll = math.random(10) - 1
                if roll == 9 then
                    creature:SummonCreature(ENTRY_OMEN, OMEN_X, OMEN_Y, OMEN_Z, OMEN_O, 7, 0)
                end
            end
        end
    end
    if creature:GetEntry() == ENTRY_CLUSTER_ELUNE then
        creature:CastSpell(creature, SPELL_LUNAR_FORTUNE)
    end
end

for _, entry in ipairs(CLUSTER_ENTRIES) do
    RegisterCreatureEvent(entry, 23, fireworkReset)
end
