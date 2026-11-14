-- Azuremyst Isle: Draenei Survivor — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (class npc_draenei_survivor : CreatureScript("npc_draenei_survivor");
-- struct npc_draenei_survivorAI : public ScriptedAI).
-- Entry: 16483 (Wowhead WotLK Classic NPC page, verified 2026-11-13;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side). Quest 9283 "Rescue the Survivors!".
-- Eluna creature events: 23 OnReset, 14 OnHitBySpell, 4 OnDied.
-- Ported arms (C++ order):
-- - Reset (zone_azuremyst_isle.cpp:88-98): DoCast(me,
--   SPELL_IRRIDATION 35046, true) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_PLAYER_CONTROLLED 0x00000008) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_IN_COMBAT 0x00080000) + SetHealth(10% of max) +
--   SetStandState(UNIT_STAND_STATE_SLEEP 3). The IsDead guard is the
--   marli ev23 idiom: Go fires ev23 on spawn, evade and death, while
--   C++ Reset runs on spawn + evade only. Initialize() timers need no
--   Lua twin — they are consumed only by the UpdateAI machine below,
--   which the pump + Reset cancel covers.
-- - SpellHit (zone_azuremyst_isle.cpp:114-127): the
--   SpellFamilyFlags[2] & 0x80000000 gate (Gift of the Naaru family)
--   via creature:GetSpellFamilyFlags(spellId, 2)
--   (lua_creature_events.go — the ev14 bare-spellId twin of the
--   procEventObject's GetSpellFamilyFlags); on pass RemoveFlag
--   (UNIT_FLAG_PLAYER_CONTROLLED) + SetStandState(STAND 0) +
--   DoCast(me, SPELL_STUNNED 28630, true) + pCaster latch +
--   SayThanksTimer 5000. A second healing hit re-arms the timer,
--   C++-verbatim (no already-awake guard).
-- - UpdateAI SayThanks/RunAway machine (zone_azuremyst_isle.cpp:
--   130-165): 1s pump (captive precedent); on SayThanks expiry
--   RemoveAura(SPELL_IRRIDATION) + Talk(SAY_HEAL 0, player) +
--   player:TalkedToCreature(entry, creature) + MoveTo(0,
--   -4115.053711, -13754.831055, 73.508949) (the C++ MovePoint
--   twin) + RunAwayTimer 10000, then Despawn() (the
--   DespawnOrUnsummon twin). A missing player nil-guards the
--   Talk/TalkedToCreature legs only (C++ GetPlayer nil check);
--   the walk + despawn still run.
-- Unmodeled (documented-only, no bridges):
-- - MoveInLineOfSight SayHelp Talk (zone_azuremyst_isle.cpp:
--   100-112): the friendly-player-within-25y Talk has no Go
--   twin — ev27 fires only from the aggro-engage branch.

local ENTRY_DRAENEI_SURVIVOR = 16483

local SPELL_IRRIDATION = 35046
local SPELL_STUNNED = 28630

local SAY_HEAL = 0

local SPELL_FAMILY_FLAGS_HEAL = 0x80000000

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PLAYER_CONTROLLED = 0x00000008
local UNIT_FLAG_IN_COMBAT = 0x00080000

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_SLEEP = 3

local RUNAWAY_X, RUNAWAY_Y, RUNAWAY_Z = -4115.053711, -13754.831055, 73.508949

local survivorState = {}

local function cancelSurvivor(guid)
    local st = survivorState[guid]
    if st ~= nil and st.event ~= nil then
        st.event:Cancel()
    end
    survivorState[guid] = nil
end

local function survivorTick(creature, guid)
    local st = survivorState[guid]
    if st == nil then
        return
    end
    if st.thanks ~= nil then
        if st.thanks <= 1000 then
            st.thanks = nil
            creature:RemoveAura(SPELL_IRRIDATION)
            local player = GetPlayerByGUID(st.playerGuid)
            if player ~= nil then
                creature:Talk(SAY_HEAL, player)
                player:TalkedToCreature(ENTRY_DRAENEI_SURVIVOR, creature)
            end
            creature:MoveTo(0, RUNAWAY_X, RUNAWAY_Y, RUNAWAY_Z)
            st.runaway = 10000
        else
            st.thanks = st.thanks - 1000
        end
        return
    end
    if st.runaway ~= nil then
        if st.runaway <= 1000 then
            cancelSurvivor(guid)
            creature:Despawn()
        else
            st.runaway = st.runaway - 1000
        end
    end
end

-- C++ npc_draenei_survivorAI::Reset (zone_azuremyst_isle.cpp:88-98).
local function onReset(event, creature)
    cancelSurvivor(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    creature:CastSpell(creature, SPELL_IRRIDATION, true)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PLAYER_CONTROLLED)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IN_COMBAT)
    creature:SetHealth(creature:GetMaxHealth() * 0.1)
    creature:SetStandState(UNIT_STAND_STATE_SLEEP)
end

-- C++ npc_draenei_survivorAI::SpellHit (zone_azuremyst_isle.cpp:114-127).
local function onSpellHit(event, creature, caster, spellId)
    if bit32.band(creature:GetSpellFamilyFlags(spellId, 2),
            SPELL_FAMILY_FLAGS_HEAL) == 0 then
        return
    end
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_PLAYER_CONTROLLED)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:CastSpell(creature, SPELL_STUNNED, true)
    local guid = creature:GetGUIDLow()
    local st = survivorState[guid]
    if st == nil then
        st = {}
        survivorState[guid] = st
        st.event = CreateLuaEvent(function()
            survivorTick(creature, guid)
        end, 1000, 0)
    end
    st.thanks = 5000
    st.runaway = nil
    if caster ~= nil then
        st.playerGuid = caster:GetGUID()
    end
end

local function onDied(event, creature)
    cancelSurvivor(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 23, onReset)
RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 4, onDied)
