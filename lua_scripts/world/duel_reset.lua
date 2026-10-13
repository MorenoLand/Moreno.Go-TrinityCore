-- DuelResetScript (PlayerScript duel lifecycle hooks) --
-- Lua port of src/server/scripts/World/duel_reset.cpp, registered from
-- AddSC_duel_reset() (call 59 in AddWorldScripts,
-- src/server/scripts/World/world_script_loader.cpp).
--
-- The C++ OnDuelStart/OnDuelEnd PlayerScript hooks are bridged by the
-- engine's PlayerEventDuelStart (10) / PlayerEventDuelEnd (11) dispatches
-- (engine/world/combat.go: the countdown-elapse arm fires
-- (event, player1, player2) like Eluna's (event, this, opponent); the
-- DuelComplete arm fires (event, winner, loser, type) matching Hooks.h).
-- The ResetDuelCooldowns / ResetDuelHealthMana config gates
-- (World.cpp:1405-1406, both default false) live inside the session
-- methods (engine/world/duel_reset.go) because the scripting runtime
-- has no world-config bridge; the gates wrap exactly the operations
-- the C++ script gates.
--
-- Ported arms (duel_reset.cpp:24-75):
-- OnDuelStart: SaveCooldownStateBeforeDuel + ResetSpellCooldowns(true)
--   (cooldowns < 10 min with > 30 s elapsed, plus the pet's full set)
--   under ResetDuelCooldowns; SaveHealthBeforeDuel + SaveManaBeforeDuel
--   + ResetAllPowers under ResetDuelHealthMana.
-- OnDuelEnd: only for DUEL_WON (type 1; INTERRUPTED = 0 and FLED = 2
--   reset nothing) — ResetSpellCooldowns(false) +
--   RestoreCooldownStateAfterDuel (duel-made >10min cooldowns carried
--   over) under ResetDuelCooldowns; RestoreHealthAfterDuel +
--   RestoreManaAfterDuel (mana only when the player's power type is
--   mana or the class is druid) under ResetDuelHealthMana.
-- Fidelity note: the C++ cooldown filter's CooldownStorage OnHold term
-- has no Go model (spellCooldown carries no OnHold), so that term is
-- always true; a missing spell DBC row is skipped instead of
-- AssertSpellInfo-aborting (the terrain.go convention).

local DUEL_WON = 1
local POWER_MANA = 0
local CLASS_DRUID = 11

local function OnDuelStart(event, player1, player2)
    player1:SaveDuelCooldownState()
    player2:SaveDuelCooldownState()
    player1:ResetDuelSpellCooldowns(true)
    player2:ResetDuelSpellCooldowns(true)

    player1:SaveDuelHealth()
    player1:SaveDuelMana()
    player1:ResetAllPowers()

    player2:SaveDuelHealth()
    player2:SaveDuelMana()
    player2:ResetAllPowers()
end

local function OnDuelEnd(event, winner, loser, duelType)
    -- duel_reset.cpp:58: do not reset anything unless DUEL_WON.
    if duelType ~= DUEL_WON then
        return
    end

    winner:ResetDuelSpellCooldowns(false)
    loser:ResetDuelSpellCooldowns(false)
    winner:RestoreDuelCooldownState()
    loser:RestoreDuelCooldownState()

    winner:RestoreDuelHealth()
    loser:RestoreDuelHealth()
    -- duel_reset.cpp:64-70: mana restore only for mana users (druids count).
    if winner:GetPowerType() == POWER_MANA or winner:GetClass() == CLASS_DRUID then
        winner:RestoreDuelMana()
    end
    if loser:GetPowerType() == POWER_MANA or loser:GetClass() == CLASS_DRUID then
        loser:RestoreDuelMana()
    end
end

RegisterPlayerEvent(10, OnDuelStart)
RegisterPlayerEvent(11, OnDuelEnd)
