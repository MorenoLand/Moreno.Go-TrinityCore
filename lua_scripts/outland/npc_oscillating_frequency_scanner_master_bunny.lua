-- Outland: Oscillating Frequency Scanner Master Bunny — Lua port of
-- src/server/scripts/Outland/zone_blades_edge_mountains.cpp
-- (class npc_oscillating_frequency_scanner_master_bunny :
-- CreatureScript("npc_oscillating_frequency_scanner_master_bunny");
-- struct npc_oscillating_frequency_scanner_master_bunnyAI : public ScriptedAI).
-- Entry: 21760 (Wowhead TBC Classic npc=21760, verified this tick; no
-- C++ entry constant exists — DB-side ScriptName binding).
-- Eluna creature events: 23 OnReset.
-- Ported arms (C++ order, zone_blades_edge_mountains.cpp:941-957):
-- - Reset: if a top bunny (NPC_OSCILLATING_FREQUENCY_SCANNER_TOP_BUNNY
--   21759) is within 25.0 yards, despawn self; else summon the top
--   bunny at self pos +0.5z (TEMPSUMMON_TIMED_OR_DEAD_DESPAWN, 50s),
--   summon the scanner GO (184926, 50s), then despawn self.
--   The IsDead guard is the marli ev23 idiom: Go fires ev23 on spawn,
--   evade and death, while C++ Reset runs on spawn + evade only.
--   Despawn() collapses C++ DespawnOrUnsummon(50s) to immediate (Go
--   Despawn takes no delay arg); the 50s window only served the
--   unported UpdateAI cast below, so no ported behavior is lost.
-- Unmodeled (documented-only, no bridges):
-- - IsSummonedBy player-capture (ev22 is pet-funnel-only) and the
--   UpdateAI 500ms/3000ms SPELL_OSCILLATION_FIELD (37408) cast on the
--   captured player — no target exists in Lua without the capture.
-- - spell_oscillating_field SpellScript (no SpellScript bridge).

local ENTRY_MASTER_BUNNY = 21760
local NPC_TOP_BUNNY = 21759
local GO_SCANNER = 184926

-- C++ npc_oscillating_frequency_scanner_master_bunnyAI::Reset
-- (zone_blades_edge_mountains.cpp:941-957): timer = 500 (feeds the
-- unported UpdateAI only) -> top-bunny check -> summon branch.
local function scannerReset(event, creature)
    if creature:IsDead() then
        return
    end
    if creature:FindNearestCreature(NPC_TOP_BUNNY, 25.0) then
        creature:Despawn()
        return
    end
    local x, y, z, o = creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO()
    creature:SummonCreature(NPC_TOP_BUNNY, x, y, z + 0.5, o, 0, 50000)
    creature:SummonGameObject(GO_SCANNER, x, y, z, o, 50)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_MASTER_BUNNY, 23, scannerReset)
