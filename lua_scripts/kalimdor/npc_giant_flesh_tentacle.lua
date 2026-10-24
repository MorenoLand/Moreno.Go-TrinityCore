-- Temple of Ahn'Qiraj: Giant Flesh Tentacle — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_cthun.cpp
-- (flesh_tentacleAI : public ScriptedAI; GetAI via
-- GetAQ40AI<flesh_tentacleAI>(creature); registered in
-- AddSC_boss_cthun()).
-- Entry: 15802 (worker-verified from C++ NPC_FLESH_TENTACLE,
-- temple_of_ahnqiraj.h:65).
-- Ported: the ctor SetCombatMovement(false) arm (boss_cthun.cpp
-- :1278-1281), wired on spawn (event 5, felmyst/muru precedent) —
-- the tentacle is stationary in C++ and never chases. Latent arm:
-- flesh tentacles only spawn from the unported body machine
-- (TRANSITION/WEAK-phase SummonCreature calls, boss_cthun.cpp
-- :656-664/:812-820), so this fires only if the creature ever
-- exists (e.g. GM spawn) until the body ports.
-- Stay-outs: the sole UpdateAI-adjacent arm, JustDied (:1283-1289)
-- -> summoner AI DoAction(ACTION_FLESH_TENTACLE_KILLED) — no
-- DoAction bridge, and the consumer (the body's weaken counter)
-- is itself unported, so relaying it would signal nothing.
-- No other arms exist in the class (no UpdateAI, no timers).
local ENTRY = 15802

local function onSpawn(_, creature)
    creature:SetCombatMovement(false)
end

RegisterCreatureEvent(ENTRY, 5, onSpawn)
