-- Azuremyst Isle: Draenei Survivor — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (class npc_draenei_survivor : CreatureScript("npc_draenei_survivor");
-- struct npc_draenei_survivorAI : public ScriptedAI).
-- Entry: 16483 (Wowhead WotLK Classic NPC page, verified 2026-11-13;
-- no entry constant in the C++ sources — the creature_template
-- ScriptName binding is DB-side). Quest 9283 "Rescue the Survivors!".
-- Eluna creature events: 23 OnReset, 14 OnHitBySpell, 4 OnDied,
-- 27 MoveInLineOfSight (proximity dispatch).
-- Ported arms (C++ order):
-- - Reset (zone_azuremyst_isle.cpp:88-98): DoCast(me,
--   SPELL_IRRIDATION 35046, true) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_PLAYER_CONTROLLED 0x00000008) + SetFlag(UNIT_FIELD_FLAGS,
--   UNIT_FLAG_IN_COMBAT 0x00080000) + SetHealth(10% of max) +
--   SetStandState(UNIT_STAND_STATE_SLEEP 3). The IsDead guard is the
--   marli ev23 idiom: Go fires ev23 on spawn, evade and death, while
--   C++ Reset runs on spawn + evade only. Initialize() timers need no
--   Lua twin — they are consumed only by the UpdateAI machine below,
--   which the one-shot timers + Reset cancel cover.
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
--   130-165): one-shot CreateLuaEvent timers (maiden schedule
--   idiom — a repeating pump cannot self-cancel: the runtime
--   Tick loop rebuilds its timer list from the iterated copies,
--   so a RemoveEventById inside a callback resurrects the
--   timer); on SayThanks expiry RemoveAura(SPELL_IRRIDATION) +
--   Talk(SAY_HEAL 0, player) + player:TalkedToCreature(entry,
--   creature) + MoveTo(0, -4115.053711, -13754.831055, 73.508949)
--   (the C++ MovePoint twin) + RunAwayTimer 10000, then
--   Despawn() (the DespawnOrUnsummon twin). A missing player
--   nil-guards the Talk/TalkedToCreature legs only (C++
--   GetPlayer nil check); the walk + despawn still run.
-- - MoveInLineOfSight SayHelp Talk (zone_azuremyst_isle.cpp:
--   100-112): rides the ev27 proximity dispatch (f40e36a0 —
--   fires for any detectable unit, in/out of combat, not just
--   the aggro-engage branch): friendly player within 25y while
--   CanSayHelp -> Talk(SAY_HELP 1, player) + SayHelpTimer 20000
--   + CanSayHelp false. The SayHelpTimer machine runs as a
--   self-re-arming one-shot chain (10s first, then 20s cycles),
--   re-initialized by Reset exactly like C++ Initialize().
-- Unmodeled (documented-only, no bridges): none — all four
-- C++ hooks (Reset / SpellHit / UpdateAI timers / MIOS) are
-- ported; the UpdateAI machine is one-shot timers, not a pump.

local ENTRY_DRAENEI_SURVIVOR = 16483

local SPELL_IRRIDATION = 35046
local SPELL_STUNNED = 28630

local SAY_HEAL = 0
local SAY_HELP = 1

local SAY_HELP_RANGE = 25.0
local SAY_HELP_INITIAL_MS = 10000
local SAY_HELP_REARM_MS = 20000

local SPELL_FAMILY_FLAGS_HEAL = 0x80000000

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_PLAYER_CONTROLLED = 0x00000008
local UNIT_FLAG_IN_COMBAT = 0x00080000

local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_SLEEP = 3

local RUNAWAY_X, RUNAWAY_Y, RUNAWAY_Z = -4115.053711, -13754.831055, 73.508949

local survivor = {}

local function cancelSurvivor(guid)
    local st = survivor[guid]
    if st ~= nil then
        if st.thanks ~= nil then
            RemoveEventById(st.thanks)
        end
        if st.runaway ~= nil then
            RemoveEventById(st.runaway)
        end
        if st.help ~= nil then
            RemoveEventById(st.help)
        end
    end
    survivor[guid] = nil
end

-- C++ SayHelpTimer machine (zone_azuremyst_isle.cpp:66-77, 154-161):
-- one-shot chain; on fire CanSayHelp goes true and the chain re-arms
-- at 20s. The callback no-ops if the state was replaced (Reset
-- rebuilds the table; SpellHit keeps it) or the creature died.
local function armHelp(guid, delay)
    local st = survivor[guid]
    if st == nil then
        return
    end
    if st.help ~= nil then
        RemoveEventById(st.help)
    end
    st.help = CreateLuaEvent(function()
        if survivor[guid] ~= st then
            return
        end
        st.help = nil
        st.canSayHelp = true
        armHelp(guid, SAY_HELP_REARM_MS)
    end, delay)
end

-- C++ npc_draenei_survivorAI::Reset (zone_azuremyst_isle.cpp:88-98).
local function onReset(event, creature)
    cancelSurvivor(creature:GetGUIDLow())
    if creature:IsDead() then
        return
    end
    local guid = creature:GetGUIDLow()
    survivor[guid] = { canSayHelp = true }
    armHelp(guid, SAY_HELP_INITIAL_MS)
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
    -- SpellHit touches only the thanks machine in C++ (the
    -- SayHelpTimer countdown keeps running); cancel thanks /
    -- runaway only and keep the help chain on the same table.
    local st = survivor[guid]
    if st == nil then
        st = { canSayHelp = true }
        survivor[guid] = st
        armHelp(guid, SAY_HELP_INITIAL_MS)
    else
        if st.thanks ~= nil then
            RemoveEventById(st.thanks)
            st.thanks = nil
        end
        if st.runaway ~= nil then
            RemoveEventById(st.runaway)
            st.runaway = nil
        end
    end
    if caster ~= nil then
        st.playerGuid = caster:GetGUID()
    end
    st.thanks = CreateLuaEvent(function()
        creature:RemoveAura(SPELL_IRRIDATION)
        local player = GetPlayerByGUID(st.playerGuid)
        if player ~= nil then
            creature:Talk(SAY_HEAL, player)
            player:TalkedToCreature(ENTRY_DRAENEI_SURVIVOR, creature)
        end
        creature:MoveTo(0, RUNAWAY_X, RUNAWAY_Y, RUNAWAY_Z)
        st.thanks = nil
        st.runaway = CreateLuaEvent(function()
            survivor[guid] = nil
            creature:Despawn()
        end, 10000)
    end, 5000)
end

local function onDied(event, creature)
    cancelSurvivor(creature:GetGUIDLow())
end

-- C++ npc_draenei_survivorAI::MoveInLineOfSight
-- (zone_azuremyst_isle.cpp:100-112): friendly player within 25y
-- while CanSayHelp -> Talk(SAY_HELP, player) + SayHelpTimer 20000
-- + CanSayHelp false. Per-tick ev27 refire is safe: the latch
-- closes after the first Talk until the help chain re-arms it.
local function onMoveInLOS(event, creature, who)
    local guid = creature:GetGUIDLow()
    local st = survivor[guid]
    if st == nil or not st.canSayHelp then
        return
    end
    if who:GetObjectType() ~= "Player" then
        return
    end
    if not creature:IsFriendlyTo(who) then
        return
    end
    if not creature:IsWithinDistInMap(who, SAY_HELP_RANGE) then
        return
    end
    creature:Talk(SAY_HELP, who)
    st.canSayHelp = false
    armHelp(guid, SAY_HELP_REARM_MS)
end

RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 23, onReset)
RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 4, onDied)
RegisterCreatureEvent(ENTRY_DRAENEI_SURVIVOR, 27, onMoveInLOS)
