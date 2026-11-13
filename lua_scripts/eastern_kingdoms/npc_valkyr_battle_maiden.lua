-- The Scarlet Enclave: Valkyr Battle-Maiden — Lua port of
-- src/server/scripts/EasternKingdoms/ScarletEnclave/
-- zone_the_scarlet_enclave.cpp (class npc_valkyr_battle_maiden :
-- CreatureScript("npc_valkyr_battle_maiden") { npc_valkyr_battle_maidenAI
-- : public PassiveAI }; AddSC_the_scarlet_enclave registers it; EK
-- loader decl :94 / call :272).
-- Entry: 28534 wowhead-verified (classic.wowhead.com/wotlk/npc=28534/
-- valkyr-battle-maiden; no NPC_ constant in the C++ tree — belnistrasz
-- precedent). The starting-zone resurrector: level 80 Elite, Revive
-- ability, "It is not yet your time, champion. Rise!" whisper, page
-- comments confirm the DK starting-zone resurrection role. Same-named
-- entries ruled out: 28487 (no Revive listed), 29111 (unknown
-- location), 31095 (Eastern Plaguelands, Touch of the Val'kyr).
-- Eluna creature events: 23 OnReset. No timers; the UpdateAI phase
-- machine stays out (see below).
-- Ported arms:
-- - Reset (C++ Reset, zone_the_scarlet_enclave.cpp:69-84):
--   SetFarVisible(true) + SetVisible(false) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_NON_ATTACKABLE 0x2) + SetCanFly(true), then GetPosition
--   and reposition (x-3.5, y-5, z+4, orientation 0.0) via Relocate
--   after a MotionMaster Clear (MoveClear twin), C++ arm order.
-- Unmodeled (documented-only, no bridges):
-- - setActive(true): no bridge — Go has no per-creature active model
--   (only FormationActive and the unmodeled BossAI setActive(false)),
--   so it is likely vacuous.
-- - UpdateAI phase machine (zone_the_scarlet_enclave.cpp:86-140):
--   phase 1 GetClosePoint (no bridge), phase 2 IsResurrectRequested
--   (no bridge) + whisper Talk(0) (broadcast Talk has no whisper
--   target) + triggered SPELL_REVIVE 51918, phase 4 DisappearAndDie
--   (no bridge). A Reset-only port is faithful state setup with no
--   revive behavior.

local ENTRY_VALKYR_BATTLE_MAIDEN = 28534

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

-- C++ npc_valkyr_battle_maidenAI::Reset (zone_the_scarlet_enclave.cpp:69-84).
local function onReset(_, creature)
    creature:SetFarVisible(true)
    creature:SetVisible(false)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:SetCanFly(true)
    local x = creature:GetX()
    local y = creature:GetY()
    local z = creature:GetZ()
    creature:MoveClear()
    creature:Relocate(x - 3.5, y - 5.0, z + 4.0, 0)
end

RegisterCreatureEvent(ENTRY_VALKYR_BATTLE_MAIDEN, 23, onReset)
