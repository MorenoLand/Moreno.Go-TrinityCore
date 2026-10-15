-- Hidden Cultist (Borean Tundra, quest 11794) --
-- Lua port of src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (npc_hidden_cultistAI — Reset shroud arm only).
-- Zone-script unit per northrend_script_loader.cpp order (decl 199 /
-- call 394; dalaran done, borean_tundra: 20 registrations).
-- Retraction re-audit of the 15:34 "CLOSED document-only" entry: that
-- audit dismissed the whole file under stale "no-timer" claims. The
-- Reset arm below is ungated and bridgeable.
-- Entries (HiddenCultist enum :1468-1470, C++-verifiable): Tom Hegger
-- 25827, Salty John Thorpe 25248, Guard Mitchells 25828 — all three
-- share this one AI.
-- Eluna creature events: 5 OnSpawn, 23 OnReset.
-- Ported arm (C++ Reset() :1508-1518): DoCast(
-- SPELL_SHROUD_OF_THE_DEATH_CULTIST 46077, :1459) — the 1-arg DoCast
-- is the non-triggered self-cast; the emote-state / npc-flags restores
-- and RestoreFaction() are DB-template idempotent (no lua bridge, no
-- visible delta). One-shot per spawn/reset, no timers, no per-GUID
-- state (the blessed_banner / storm_cloud shape).
-- Unmodeled: the OnGossipSelect -> DoAction(1) -> uiEventPhase Talk
-- chain (:1651-1660 -> :1519 -> :1546-1605) — the gossip seed IS
-- bridged (creature_gossip, nefarian precedent) but the machine
-- terminates in AttackPlayer() (SetFaction(FACTION_MONSTER) +
-- AttackStart(player), :1541-1545) with no faction/attack bridges;
-- porting the Talks without the ambush would strand the quest, so the
-- whole chain stays documented-only (the rageclaw stranded-half
-- precedent). Tom Hegger's Talk(SAY_HIDDEN_CULTIST_3, player) would
-- additionally collapse to broadcast (torturer_lecraft precedent).
-- Go: RegisterLuaBoss("npc_hidden_cultist", <each entry>) — the no-op
-- AI keeps these quest NPCs passive pre-ambush (storm_cloud /
-- blessed_banner zone-script precedent).

local SPELL_SHROUD_OF_THE_DEATH_CULTIST = 46077

local ENTRIES_HIDDEN_CULTIST = { 25827, 25248, 25828 }

-- C++ Reset() :1514 — non-triggered self-cast of the shroud visual,
-- one-shot per spawn / reset.
local function hiddenCultistSpawnOrReset(event, creature)
    -- C++ npc_hidden_cultistAI::Reset(): DoCast(SPELL_SHROUD_OF_THE_DEATH_CULTIST)
    -- (zone_borean_tundra.cpp). Alive-gated so the death-leg ev23 fire is not phantom.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SHROUD_OF_THE_DEATH_CULTIST)
    end
end

for _, entry in ipairs(ENTRIES_HIDDEN_CULTIST) do
    RegisterCreatureEvent(entry, 5, hiddenCultistSpawnOrReset)
    RegisterCreatureEvent(entry, 23, hiddenCultistSpawnOrReset)
end
