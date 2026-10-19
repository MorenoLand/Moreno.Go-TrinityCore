-- Tesla Coil (Naxxramas, Thaddius room) — Lua port of the npc_tesla
-- slice of src/server/scripts/Northrend/Naxxramas/boss_thaddius.cpp
-- (npc_tesla via GetNaxxramasAI<npc_teslaAI> (ScriptedAI);
-- AddSC_boss_thaddius registers it alongside boss_thaddius,
-- npc_stalagg, npc_feugen, the three polarity/magnetic-pull
-- SpellScripts, at_thaddius_entrance and achievement_thaddius_
-- shocking; loader decl 73 / call 268 per
-- northrend_script_loader.cpp — the FIFTEENTH Naxxramas group in
-- AddNorthrendScripts(), immediately after AddSC_boss_gothik()).
-- Entry: 16218 Tesla Coil (naxxramas.h NPC_TESLA line 101 —
-- kalecgos pass; instance_naxxramas.cpp binds the tesla GUIDs
-- (line 171); the CreatureScript ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "npc_tesla"
-- hits boss_thaddius.cpp only (loader carries only the decl/call
-- lines); zero sql/ hits. No tesla lua existed.
-- Eluna creature events: 9 OnDamageTaken. The C++ AI is otherwise
-- inert: EnterEvadeMode/UpdateAI/JustEngagedWith are empty
-- overrides (boss_thaddius.cpp:970-973); the coil only talks
-- (EMOTE_TESLA_OVERLOAD / EMOTE_TESLA_LINK_BREAKS via the unbridged
-- cross-AI stalagg/feugen beam machines) and channels the tesla
-- shock visuals (unbridged).
-- Ported arms (C++-exact):
-- - DamageTaken (boss_thaddius.cpp:974): damage = 0
--   unconditionally ("no, you can't kill it"). Event 9 consumes the
--   second return as the damage rewrite (fireCreatureDamageTaken;
--   halazzi precedent), so the handler returns false, 0.
-- Unmodeled (documented-only, no bridges): the shock-visual
-- channeling + beam talk legs (cross-AI DATA_THADDIUS / coil
-- CastStop + Talk via the stalagg/feugen AIs — no cross-AI
-- bridge); the ACTION_TRANSITION coil GO GO_STATE_READY leg (no
-- GO-state bridge).

local ENTRY_TESLA = 16218

-- C++ npc_tesla::DamageTaken (boss_thaddius.cpp:974): damage = 0.
local function teslaDamageTaken(event, creature, attacker, damage)
    return false, 0
end

RegisterCreatureEvent(ENTRY_TESLA, 9, teslaDamageTaken)
