-- Battle for Mount Hyjal: Kazrogal — Lua port of
-- src/server/scripts/Kalimdor/CavernsOfTime/BattleForMountHyjal/
-- boss_kazrogal.cpp (246 lines; boss_kazrogalAI : public hyjal_trashAI :
-- public EscortAI; spell_mark_of_kazrogal SpellScriptLoader;
-- AddSC_boss_kazrogal at end registers both; kalimdor loader decl 32 /
-- call 145 per kalimdor_script_loader.cpp).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole source
-- of "boss_kazrogal" and "spell_mark_of_kazrogal" (loader lines only
-- otherwise).
-- Entry: hyjal.h HYCreaturesIds names KAZROGAL = 17888 under the "Bosses
-- summoned after every 8 waves" comment AND instance_hyjal.cpp:122 cases
-- it in OnCreatureCreate (Kazrogal GUID capture, :156 the DATA_KAZROGAL
-- GetGuidData leg) — the name-to-entry tie is C++-verified (ramstein
-- strength); the creature_template ScriptName binding stays DB-side.
-- GetAI uses GetHyjalAI<boss_kazrogalAI>, same as jaina/thrall.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs; the IsEvent escort
-- machine needs an EscortAI + instance bridge and has none — see below):
-- - Engage (C++ JustEngagedWith): SetBossState(DATA_KAZROGAL 8,
--   IN_PROGRESS) before Talk SAY_ONAGGRO (2) (boss_kazrogal.cpp:90-94)
--   + arm each timer at its C++ constructor-init cooldown (hyjal.lua
--   convention); MarkTimerBase reset to 45000 on engage (C++
--   Initialize() leg — the timer-reset half of Reset collapses into the
--   engage re-arm, hyjal.lua convention).
-- - Cleave 31436 self-cast (C++ DoCast(me)), non-triggered, init 5s ->
--   {6s,21s} (C++ 6000 + rand32()%15000, jeklik convention).
-- - War Stomp 31480 self-cast (C++ DoCast(me)), non-triggered, init 15s
--   -> 60s.
-- - Mark 31447 non-triggered self-cast (C++ DoCastAOE resolves to the
--   caster's own self-cast — illidan precedent), init 45s; re-arm is the
--   C++-verbatim decreasing base: MarkTimerBase -= 5000, floor 5500,
--   MarkTimer = MarkTimerBase — so 45s, 40s, 35s, 30s, 25s, 20s, 15s,
--   10s, then 5500 forever; Talk SAY_MARK (1) unconditionally on each
--   expiry (C++ talks after the cast with no nil-target leg — AOE cast).
-- - KilledUnit (event 3, wired — nalorakk DISCOVERY holds): Talk
--   SAY_ONSLAY (0); no TYPEID gate in C++ (shade_of_aran precedent).
-- - Death (C++ JustDied): SetBossState(DATA_KAZROGAL 8, DONE) after
--   hyjal_trashAI::JustDied, before DoPlaySoundToSet(me, 11018) (:113-118);
--   hyjal_trashAI::JustDied is instance bookkeeping (blocked, standing);
--   DoPlaySoundToSet(me, 11018) is ported via creature:PlayDirectSound
--   in onDied.
-- Unmodeled (documented-only, no bridges):
-- - The IsEvent escort machine: the go-flagged first-UpdateAI block adds
--   8 C++-verbatim waypoints (5492.91,-2404.61,1462.63 /
--   5531.76,-2460.87,1469.55 / 5554.58,-2514.66,1476.12 /
--   5554.16,-2567.23,1479.90 / 5540.67,-2625.99,1480.89 /
--   5508.16,-2659.20,1480.15 / 5489.62,-2704.05,1482.18 /
--   5457.04,-2726.26,1485.10), Start(false, true),
--   SetDespawnAtEnd(false); WaypointReached(7) AddThreat(0) on the
--   instance DATA_THRALL GUID — EscortAI movement plus instance-script
--   model, both blocked (standing; the whole hyjal_trashAI escort/wave
--   machine is blocked the same way, see hyjal_trash.lua).
-- - SetBossState legs PORTED this tick (bridge live
--   lua_creature_events.go:5917; the DATA_*EVENT slot 9 is the C++
--   encounter-array id, the DATA id 8 is the Go boss-state key —
--   ruby-sanctum convention): Reset writes NOT_STARTED (:84-87,
--   alive-gated, NO != DONE gate — these AIs are hyjal_trashAI /
--   EscortAI, not BossAI, so no _Reset gate applies; EscortAI::
--   EnterEvadeMode calls Reset(), ScriptedEscortAI.cpp:114, so evade
--   writes NOT_STARTED too, never FAIL — no ev2/ev24 leg). The C++
--   `if (IsEvent)` gate collapses: the entry only ever fights as the
--   event boss (summoned by hyjalAI::SummonCreature with IsEvent=true,
--   hyjalAI.cpp:520-523), same as the combat arms ported unconditionally.
-- - hyjal_trashAI::JustDied: instance->SetData(DATA_TRASH, 0) wave signal
--   plus the MINRAIDDAMAGE lootable-flag gate — blocked (standing).
-- - spell_mark_of_kazrogal SpellScriptLoader: the SpellScript
--   OnObjectAreaTargetSelect filter (targets.remove_if — keeps only mana
--   users, MarkTargetFilter) needs a SpellScript target-filter bridge
--   and stays out; the AuraScript OnEffectPeriodic handler (if target
--   mana == 0, target->CastSpell(target, SPELL_MARK_DAMAGE = 31463,
--   aurEff) and SetDuration(0)) is PORTED below via RegisterAuraEvent
--   (tick 194) — the no-AuraScript-bridge block is stale (tick 188).

local ENTRY = 17888

local DATA_KAZROGAL = 8 -- hyjal.h:36

-- EncounterState (InstanceScript.h:68-76): NOT_STARTED 0, IN_PROGRESS 1, DONE 3.
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3

local SAY_ONSLAY  = 0
local SAY_MARK    = 1
local SAY_ONAGGRO = 2

local SPELL_CLEAVE    = 31436
local SPELL_WARSTOMP  = 31480
local SPELL_MARK      = 31447

local timers = {}
local markBase = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay, 1)
end

-- C++ EVENT_CLEAVE: DoCast(me, 31436), non-triggered, init 5s ->
-- {6s,21s}.
-- C++ re-arm is 6000 + rand32() % 15000 -> [6000, 20999] (exclusive
-- top edge), NOT urand(6000, 21000); the 20999 is C++-exact.
local function onCleave(creature, guid)
    creature:CastSpell(creature, SPELL_CLEAVE)
    schedule(guid, "cleave", math.random(6000, 20999), function()
        onCleave(creature, guid)
    end)
end

-- C++ EVENT_WARSTOMP: DoCast(me, 31480), non-triggered, init 15s -> 60s.
local function onWarStomp(creature, guid)
    creature:CastSpell(creature, SPELL_WARSTOMP)
    schedule(guid, "warstomp", 60000, function()
        onWarStomp(creature, guid)
    end)
end

-- C++ EVENT_MARK: DoCastAOE(31447) (non-triggered self-cast, illidan
-- precedent), init 45s; re-arm is the C++-verbatim decreasing base —
-- MarkTimerBase -= 5000, floored at 5500, MarkTimer = MarkTimerBase;
-- Talk SAY_MARK fires unconditionally on expiry.
local function onMark(creature, guid)
    creature:CastSpell(creature, SPELL_MARK)
    local base = markBase[guid] - 5000
    if base < 5500 then
        base = 5500
    end
    markBase[guid] = base
    creature:Talk(SAY_MARK)
    schedule(guid, "mark", base, function()
        onMark(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    markBase[guid] = 45000
    -- C++ JustEngagedWith (:90-94): SetData(DATA_KAZROGALEVENT,
    -- IN_PROGRESS) first, then Talk.
    creature:SetBossState(DATA_KAZROGAL, BOSS_STATE_IN_PROGRESS)
    creature:Talk(SAY_ONAGGRO)
    schedule(guid, "cleave", 5000, function()
        onCleave(creature, guid)
    end)
    schedule(guid, "warstomp", 15000, function()
        onWarStomp(creature, guid)
    end)
    schedule(guid, "mark", 45000, function()
        onMark(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUIDLow())
    -- C++ Reset (:84-87): Initialize() then SetData(NOT_STARTED).
    -- Alive-gated: Go fires ev23 on the death path before ev4
    -- (moroes/algalon precedent) and C++ Reset never runs on death.
    -- NO != DONE gate (see header): this AI is hyjal_trashAI /
    -- EscortAI, not BossAI.
    if creature:IsAlive() then
        creature:SetBossState(DATA_KAZROGAL, BOSS_STATE_NOT_STARTED)
    end
end

-- C++ JustDied (:113-118): hyjal_trashAI::JustDied first, then
-- SetData(DONE), then DoPlaySoundToSet(me, SOUND_ONDEATH 11018) =
-- creature:PlayDirectSound (SMSG_PLAY_SOUND).
local function onDied(event, creature, killer)
    creature:SetBossState(DATA_KAZROGAL, BOSS_STATE_DONE)
    creature:PlayDirectSound(11018)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, function(event, creature, victim)
    creature:Talk(SAY_ONSLAY)
end)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- C++ spell_mark_of_kazrogal::OnPeriodic
-- (boss_kazrogal.cpp:213-222): EFFECT_0 SPELL_AURA_PERIODIC_MANA_
-- LEECH. No PreventDefaultAction, so the Go default leech tick
-- still fires (it drains 0 at zero mana anyway). When the target's
-- mana hits 0 the target casts SPELL_MARK_DAMAGE 31463 on itself,
-- triggered by the aura (the aurEff overload = triggered cast),
-- and SetDuration(0) removes the aura (aura:SetDuration(0)
-- twin). POWER_MANA is 0. The SpellScript target filter (mana
-- users only) stays out — no SpellScript target-filter bridge.
local SPELL_MARK_DAMAGE = 31463
local POWER_MANA = 0

local function markOfKazrogalTick(event, aura, target)
    if target:GetPower(POWER_MANA) == 0 then
        target:CastSpell(target, SPELL_MARK_DAMAGE, true)
        aura:SetDuration(0)
    end
end

RegisterAuraEvent(SPELL_MARK, 1, markOfKazrogalTick)
