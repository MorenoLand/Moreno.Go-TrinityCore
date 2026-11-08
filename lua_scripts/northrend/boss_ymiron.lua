-- King Ymiron (Utgarde Pinnacle) — Lua port of
-- src/server/scripts/Northrend/UtgardeKeep/UtgardePinnacle/boss_ymiron.cpp
-- (355 lines; 3 scripts — boss_ymiron (CreatureScript via
-- GetUtgardePinnacleAI (BossAI), DATA_KING_YMIRON = 3);
-- spell_dark_slash (bare SpellScript via RegisterSpellScript, SpellScript
-- OnHit -> CalculateDamage half-target-health); achievement_kings_bane
-- (AchievementCriteriaScript, OnCheck GetData(DATA_KINGS_BANE));
-- all registered from inside AddSC_boss_ymiron(); loader decl 135 /
-- call 330 per northrend_script_loader.cpp — the FOURTH group of the "//
-- Utgarde Keep - Utgarde Pinnacle" block in AddNorthrendScripts(),
-- immediately after AddSC_boss_skadi() (call 329); the checkpoint
-- sequence (boss_skadi -> boss_ymiron) is followed).
-- Entries: 26861 King Ymiron (utgarde_pinnacle.h NPC_KING_YMIRON,
-- line 56) — entry-verifiable, registration proceeds (the
-- nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- DATA_KING_YMIRON = 3 (utgarde_pinnacle.h line 34).
-- Sole-source verified: whole-server-tree grep for "AddSC_boss_ymiron"
-- hits boss_ymiron.cpp only (+ the loader decl/call lines); this clone
-- carries no sql/ tree, so ScriptName bindings are DB-side by
-- construction. The lua was ported Oct 2 (Talk-only, stale
-- "no-timer-bridge" doctrine) and UPGRADED this run under the
-- CreateLuaEvent doctrine (engine/scripting/runtime.go:798 — the
-- maiden_of_virtue / keleseth / yogg_saron precedent): module-local
-- GUID-keyed timers table, wipe on events 2 (leave combat), 4 (death),
-- 23 (evade). creature:CastSpell(nil, spell) = DoCastVictim;
-- creature:CastSpell(creature, spell) = DoCastSelf / DoCastAOE (the
-- selin_fireheart / mr_smite / boss_ai.go DoCastAOE lowering precedent);
-- the 3rd CastSpell arg is the C++ DoCast 3rd arg (2-arg / 1-arg
-- overloads = non-triggered); {minMs, maxMs} table delay = randtime
-- range, resolved in schedule() so the concrete due time is recorded
-- for the DelayEvents twin (thekal armEntry idiom). The 2026-10-02
-- "no-timer-bridge" header doctrine is RETRACTED
-- for this file: CreateLuaEvent is a real registered bridge, so the
-- ungated timer arms below are ported rather than parked.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnKill,
-- 4 OnDied, 9 OnDamageTaken, 19 OnSummoned, 23 OnReset, 28
-- OnMovementInform.
-- Ported arms (C++-exact for all modeled arms; line refs are
-- boss_ymiron.cpp):
-- JustEngagedWith (:141-149, base-first) — BossAI::JustEngagedWith
-- arms SetBossState(DATA_KING_YMIRON 3, IN_PROGRESS) PORTED tick 261 via
-- creature:SetBossState (lua_creature_events.go:5917) + SetCombatPulseDelay(5)
-- + me-form DoZoneInCombat (ScriptedCreature.cpp:537/540/542) ported
-- before Talk(SAY_AGGRO 0) (event 1; instance/setActive/ScheduleTasks
-- legs out) — plus the four EVENT_GROUP_BASE_SPELLS timer inits
-- carried on event 1 on the xt002 precedent (C++ schedules on
-- JustEngagedWith, Go on event 1, same inits). All four arms run
-- under BossAI::UpdateAI base gates (ScriptedCreature.cpp pre-loop
-- + in-loop CASTING returns; no UpdateAI override in the file), so
-- all four carry the 500ms IsCasting defer (no phase conditions on
-- the base spells; the event-group delay legs ride the unbridged
-- boat machine — see DEVIATION):
--   EVENT_BANE (arm :236-238): DoCast(SPELL_BANE 48294), 1-arg ->
--   DoCastAOE/self-cast -> CastSpell(creature, 48294), non-triggered;
--   init {18s,23s}, re-arm {20s,25s}.
--   EVENT_FETID_ROT (arm :240-242): DoCastVictim(SPELL_FETID_ROT 48291),
--   1-arg = non-triggered -> CastSpell(nil, 48291); init {8s,13s},
--   re-arm {10s,15s}.
--   EVENT_DARK_SLASH (arm :244-246): DoCastVictim(SPELL_DARK_SLASH
--   48292), 1-arg = non-triggered -> CastSpell(nil, 48292); init
--   {28s,33s}, re-arm {30s,35s}.
--   EVENT_ANCESTORS_VENGEANCE (arm :248-250): DoCast(me,
--   SPELL_ANCESTORS_VENGEANCE 16939), 2-arg = non-triggered ->
--   CastSpell(creature, 16939); init DUNGEON_MODE(60s,45s), re-arm
--   DUNGEON_MODE({60s,65s},{45s,50s}) — difficulty-branched via
--   creature:IsHeroic() (lua_creature_events.go:260, per-instance
--   difficulty; the old "no difficulty bridge" claim was stale).
-- JustDied (:277) — _JustDied() SetBossState(DATA_KING_YMIRON 3, DONE)
-- PORTED tick 261 via creature:SetBossState (lua_creature_events.go:5917),
-- first per C++ :279; Talk(SAY_DEATH 2) — unconditional (the sjonnir
-- JustDied-Talk precedent; the events.Reset / scheduler.CancelAll legs
-- are internal).
-- Reset (:134-141) — _Reset() SetBossState(DATA_KING_YMIRON 3, NOT_STARTED)
-- unless DONE + SetReactState(REACT_AGGRESSIVE) PORTED tick 261 on a NEW
-- ev23 handler split from ev2 (the sjonnir precedent); Initialize() legs
-- have no Lua mirrors (see ymironReset).
-- KilledUnit (:283) — Talk(SAY_SLAY 1) player-gated (event 3 — the
-- razuvious player-gated variant precedent).
-- PORTED (tick 483) — the ancestor phase machine (:165-271, :208-230),
-- unblocked by the MoveTo + ev28 MovementInform bridges that landed
-- after the tick-261 header was written (MoveTo lua_creature_events.go:
-- 4838 fires MovementInform(POINT_MOTION_TYPE 8, id) on ev28,
-- creaturemotion.go:5725; movementGeneratorPoint = 8,
-- creaturemotion.go:4020; algalon outro2 is the live ev28 precedent):
-- DamageTaken health-threshold latch (ev9, nefarian postDamage > 0
-- guard form) -> InterruptNonMeleeSpells + SCREAMS self-cast +
-- AttackStop + REACT_PASSIVE + MoveTo(POINT_BOAT 0, boat coords) +
-- DespawnBoatGhosts (Lua-kept summon refs, mimiron precedent) +
-- CancelEvent(prev ancestor event) + DelayEvents(10s, base group) via
-- the thekal armEntry/delayTimers twin (entries carry due/fn; nowMs =
-- GetCurrTime) + ActivedNumber = Order; MovementInform (ev28, type 8 /
-- id 0, IsInCombat + inBoat stale-arrival guards) -> Talk(summon say
-- 3-6) + SummonCreature(ancestor visual, TEMPSUMMON_CORPSE_DESPAWN 4)
-- + DoCast(ancestor, CHANNEL_YMIRON_TO_SPIRIT 48307) +
-- ancestor->CastSpell(me, CHANNEL_SPIRIT_TO_YMIRON 48316, triggered)
-- + SetFlag(59, NON_ATTACKABLE | NOT_SELECTABLE) + SetDisableGravity +
-- EVENT_RESUME_COMBAT 5s later (REACT_AGGRESSIVE + ancestor event 5s
-- later); the four ancestor events — BJORN one-shot SUMMON_SPIRIT_
-- FOUNT 48386, HALDOR DoCastVictim SPIRIT_STRIKE 48423 5s, RANULF
-- DoCast self SPIRIT_BURST 48529 10s, TORGYN 4x SUMMON_AVENGING_
-- SPIRIT 48592 15s — all with the 500ms IsCasting defer (BossAI base
-- gates); JustSummoned (ev19 — spell summons ride summonTempCreature,
-- which fires ev19, lua_creature_events.go:190): SPIRIT_FOUNT ->
-- self-cast 48380 triggered + SetDisplayId 11686 + GUID latch,
-- AVENGING_SPIRIT -> random-threat-target AddThreat(0) + AttackStart
-- (archavon randomThreatTarget precedent); ActiveOrder shuffled once
-- per AI instance (ctor :116-130, NOT on Reset — Fisher-Yates via
-- math.random, kept across ev23, dropped on ev4); Reset/JustDied
-- despawn the boat ghosts (_Reset/_JustDied summons.DespawnAll(),
-- ScriptedCreature.cpp:507/:516) and reset modifier/active.
-- Heroic gives 3 boat phases (75/50/25 — the 4th threshold, 0, can
-- never fire on non-lethal damage); normal gives 4 (80/60/40/20).
-- DORMANT until Spell.dbc ships the channel/summon spell records +
-- creature_template rows for the visual/fount/spirit entries (the
-- summons return nil without them, like every other spell-summon
-- leg in this tree).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 26861):
-- spell_dark_slash — SpellScript OnHit damage hook only; joins the
-- no-SpellScript-bridge queue.
-- The kingsBane SpellHitTarget latch (:153-157): if (target->GetTypeId()
-- == TYPEID_PLAYER && spellInfo->Id == SPELL_BANE_HIT 59203) kingsBane =
-- false — PORTED tick 417: the latch is mirrored into the boss motion's
-- SetData store (Reset true below / ymironSpellHitTarget clear) and read
-- by the Go gate via bossLatchData. Event 15 fires on the caster when
-- its spell hits a unit (boss_ai.go:4011); the 59203 delivery is
-- DBC-driven (no Bane SpellScript in the file).
-- achievement_kings_bane (OnCheck: GetData(DATA_KINGS_BANE 2157) == 1) —
-- PORTED tick 417 (SetData mirror); see the Go gate.

local ENTRY_KING_YMIRON = 26861
local DATA_KING_YMIRON = 3 -- utgarde_pinnacle.h:34
local DATA_KINGS_BANE = 2157 -- boss_ymiron.cpp:99

local SAY_AGGRO = 0
local SAY_SLAY = 1
local SAY_DEATH = 2

local SPELL_BANE = 48294
local SPELL_BANE_HIT = 59203 -- boss_ymiron.cpp:30 (checked for King's Bane)
local SPELL_FETID_ROT = 48291
local SPELL_DARK_SLASH = 48292
local SPELL_ANCESTORS_VENGEANCE = 16939

local SPELL_SCREAMS_OF_THE_DEAD = 51750
local SPELL_CHANNEL_YMIRON_TO_SPIRIT = 48307
local SPELL_CHANNEL_SPIRIT_TO_YMIRON = 48316
local SPELL_SUMMON_SPIRIT_FOUNT = 48386
local SPELL_SPIRIT_FOUNT = 48380
local SPELL_SPIRIT_STRIKE = 48423
local SPELL_SPIRIT_BURST = 48529
local SPELL_SUMMON_AVENGING_SPIRIT = 48592

local NPC_SPIRIT_FOUNT = 27339
local NPC_AVENGING_SPIRIT = 27386

local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36
local POINT_BOAT = 0 -- boss_ymiron.cpp enum MovePoints (sole member)

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000

-- C++ ActiveBoat[4] (boss_ymiron.cpp:105-111): {npc, say, MoveX/Y/Z,
-- SpawnX/Y/Z/O, event}. Lua 1-based array; the order values below are
-- the C++ 0-based boat indices (ActiveOrder entries).
local ACTIVE_BOAT = {
    { npc = 27304, say = 3, mx = 404.379, my = -335.335, mz = 104.756, sx = 413.594, sy = -335.408, sz = 107.995, so = 3.157, key = "bjorn" }, -- Bjorn
    { npc = 27310, say = 4, mx = 380.813, my = -335.069, mz = 104.756, sx = 369.994, sy = -334.771, sz = 107.995, so = 6.232, key = "haldor" }, -- Haldor
    { npc = 27311, say = 5, mx = 381.546, my = -314.362, mz = 104.756, sx = 370.841, sy = -314.426, sz = 107.995, so = 6.232, key = "ranulf" }, -- Ranulf
    { npc = 27312, say = 6, mx = 404.310, my = -314.761, mz = 104.756, sx = 413.992, sy = -314.703, sz = 107.995, so = 3.157, key = "torgyn" }, -- Torgyn
}

local timers = {}

-- GetCurrTime is wall-clock ms (Eluna global, engine globals.go:941);
-- os.clock() is CPU time and stalls while the server idles, which would
-- corrupt the due/remaining math used by delayTimers (thekal idiom).
local function nowMs()
    return GetCurrTime()
end

local function armEntry(guid, key, delayMs, fn)
    local entry = { due = nowMs() + delayMs, fn = fn }
    entry.id = CreateLuaEvent(function()
        local per = timers[guid]
        if per and per[key] == entry then
            per[key] = nil
        end
        fn()
    end, delayMs)
    return entry
end

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, entry in pairs(per) do
            RemoveEventById(entry.id)
        end
        timers[guid] = nil
    end
end

local function cancelTimerKey(guid, key)
    local per = timers[guid]
    if per and per[key] then
        RemoveEventById(per[key].id)
        per[key] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key].id)
    end
    -- {minMs, maxMs} table delay = randtime range, resolved here so the
    -- concrete due time is recorded for delayTimers.
    local delayMs = delay
    if type(delay) == "table" then
        delayMs = math.random(delay[1], delay[2])
    end
    per[key] = armEntry(guid, key, delayMs, fn)
end

-- C++ EventMap::DelayEvents (thekal idiom): push the named armed timers
-- out by extraMs, preserving each timer's remaining time.
local function delayTimers(guid, keys, extraMs)
    local per = timers[guid]
    if not per then
        return
    end
    local now = nowMs()
    for _, key in ipairs(keys) do
        local entry = per[key]
        if entry then
            RemoveEventById(entry.id)
            local remaining = entry.due - now
            if remaining < 0 then
                remaining = 0
            end
            per[key] = armEntry(guid, key, remaining + extraMs, entry.fn)
        end
    end
end

local boat = {}

-- C++ boss_ymironAI ctor (:116-130): ActiveOrder is a random permutation
-- of {0,1,2,3} shuffled ONCE per AI instance (the comment leaves it out
-- of Initialize() deliberately — it does NOT reshuffle on Reset). Lua:
-- shuffle lazily on first sight of a GUID, keep across ev23 resets,
-- drop on death (ev4) so the next spawn reshuffles. modifier =
-- HealthAmountModifier (starts 1), mult = DUNGEON_MODE(20,25),
-- active = ActivedNumber+1 (1-based slot into order), ancestor/fount =
-- Lua-kept summon refs (mimiron precedent), ancestorKey = armed
-- ancestor-event key (or nil), inBoat = spline in flight.
local function boatStateFor(guid, heroic)
    local st = boat[guid]
    if st then
        return st
    end
    local order = { 0, 1, 2, 3 }
    for i = 1, 3 do
        local r = math.random(i, 4)
        order[i], order[r] = order[r], order[i]
    end
    st = {
        order = order,
        modifier = 1,
        mult = heroic and 25 or 20,
        active = 1,
        ancestor = nil,
        fount = nil,
        ancestorKey = nil,
        inBoat = false,
    }
    boat[guid] = st
    return st
end

local function despawnBoatGhosts(st)
    -- C++ DespawnBoatGhosts (:290-297): DisappearAndDie on the stored
    -- GUIDs, then clear. Twin: DespawnOrUnsummon(1) on the Lua-kept
    -- refs (algalon black-hole precedent).
    if st.ancestor then
        st.ancestor:DespawnOrUnsummon(1)
        st.ancestor = nil
    end
    if st.fount then
        st.fount:DespawnOrUnsummon(1)
        st.fount = nil
    end
end

-- Shared timer wipe for events 2/23/4.
local function cancelCombatTimers(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ EVENT_BANE arm (:236-238): DoCast(SPELL_BANE 48294), 1-arg =
-- non-triggered -> self-cast; init {18s,23s}, re-arm {20s,25s}.
local function baneTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bane", 500, function()
            baneTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BANE)
    schedule(guid, "bane", {20000, 25000}, function()
        baneTick(creature, guid)
    end)
end

-- C++ EVENT_FETID_ROT arm (:240-242): DoCastVictim(SPELL_FETID_ROT
-- 48291), 1-arg = non-triggered; init {8s,13s}, re-arm {10s,15s}.
local function fetidRotTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fetidrot", 500, function()
            fetidRotTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_FETID_ROT)
    schedule(guid, "fetidrot", {10000, 15000}, function()
        fetidRotTick(creature, guid)
    end)
end

-- C++ EVENT_DARK_SLASH arm (:244-246): DoCastVictim(SPELL_DARK_SLASH
-- 48292), 1-arg = non-triggered; init {28s,33s}, re-arm {30s,35s}.
local function darkSlashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "darkslash", 500, function()
            darkSlashTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_DARK_SLASH)
    schedule(guid, "darkslash", {30000, 35000}, function()
        darkSlashTick(creature, guid)
    end)
end

-- C++ EVENT_ANCESTORS_VENGEANCE arm (:248-250): DoCast(me,
-- SPELL_ANCESTORS_VENGEANCE 16939), 2-arg = non-triggered; init
-- DUNGEON_MODE(60s,45s), re-arm DUNGEON_MODE({60s,65s},{45s,50s}),
-- branched on creature:IsHeroic().
local function ancestorsVengeanceTick(creature, guid, heroic)
    if creature:IsCasting() then
        schedule(guid, "ancestorsvengeance", 500, function()
            ancestorsVengeanceTick(creature, guid, heroic)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_ANCESTORS_VENGEANCE)
    local delay = heroic and {45000, 50000} or {60000, 65000}
    schedule(guid, "ancestorsvengeance", delay, function()
        ancestorsVengeanceTick(creature, guid, heroic)
    end)
end

-- C++ EVENT_BJORN_SPIRIT_FOUNT (:256-258): DoCast(SPELL_SUMMON_SPIRIT_
-- FOUNT 48386) — self-cast; one-shot, no re-arm. 500ms IsCasting defer
-- (BossAI base gates, as the base spells).
local function bjornTick(creature, guid, st)
    if creature:IsCasting() then
        schedule(guid, "bjorn", 500, function()
            bjornTick(creature, guid, st)
        end)
        return
    end
    st.ancestorKey = nil
    creature:CastSpell(creature, SPELL_SUMMON_SPIRIT_FOUNT)
end

-- C++ EVENT_HALDOR_SPIRIT_STRIKE (:259-262): DoCastVictim(SPELL_SPIRIT_
-- STRIKE 48423); re-arm 5s.
local function haldorTick(creature, guid, st)
    if creature:IsCasting() then
        schedule(guid, "haldor", 500, function()
            haldorTick(creature, guid, st)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_SPIRIT_STRIKE)
    schedule(guid, "haldor", 5000, function()
        haldorTick(creature, guid, st)
    end)
end

-- C++ EVENT_RANULF_SPIRIT_BURST (:263-266): DoCast(me, SPELL_SPIRIT_
-- BURST 48529); re-arm 10s.
local function ranulfTick(creature, guid, st)
    if creature:IsCasting() then
        schedule(guid, "ranulf", 500, function()
            ranulfTick(creature, guid, st)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_SPIRIT_BURST)
    schedule(guid, "ranulf", 10000, function()
        ranulfTick(creature, guid, st)
    end)
end

-- C++ EVENT_TORGYN_SUMMON_AVENGING_SPIRITS (:267-271): 4x DoCast
-- (SPELL_SUMMON_AVENGING_SPIRIT 48592); re-arm 15s.
local function torgynTick(creature, guid, st)
    if creature:IsCasting() then
        schedule(guid, "torgyn", 500, function()
            torgynTick(creature, guid, st)
        end)
        return
    end
    for _ = 1, 4 do
        creature:CastSpell(creature, SPELL_SUMMON_AVENGING_SPIRIT)
    end
    schedule(guid, "torgyn", 15000, function()
        torgynTick(creature, guid, st)
    end)
end

local ANCESTOR_TICKS = {
    bjorn = bjornTick,
    haldor = haldorTick,
    ranulf = ranulfTick,
    torgyn = torgynTick,
}

-- C++ EVENT_RESUME_COMBAT (:252-255): REACT_AGGRESSIVE + the active
-- ancestor's event 5s later.
local function resumeCombat(creature, guid, st)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
    local b = ACTIVE_BOAT[st.order[st.active] + 1]
    st.ancestorKey = b.key
    schedule(guid, b.key, 5000, function()
        ANCESTOR_TICKS[b.key](creature, guid, st)
    end)
end

-- C++ boss_ymironAI::DamageTaken (:208-230): the health-threshold latch.
-- HealthBelowPctDamaged(100 - DUNGEON_MODE(20,25) * HealthAmountModifier,
-- damage) is strict less-than (Unit.h:873) on post-damage health, and the
-- latch never fires on the killing blow (!(damage >= GetHealth())) — the
-- nefarian postDamage > 0 guard form. Order = HealthAmountModifier - 1,
-- then the modifier increments; the next latch reads the next threshold.
-- Heroic mult 25 gives thresholds 75/50/25 (the 4th, 0, can never fire —
-- post-damage health is > 0); normal mult 20 gives 80/60/40/20.
-- C++ order: InterruptNonMeleeSpells + SCREAMS self-cast + AttackStop +
-- REACT_PASSIVE + MovePoint(POINT_BOAT) + DespawnBoatGhosts x2 +
-- CancelEvent(prev ancestor event) + DelayEvents(10s, base group) +
-- ActivedNumber = Order.
local function ymironDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = boatStateFor(guid, creature:IsHeroic())
    local postDamage = creature:GetHealth() - damage
    if postDamage <= 0 then
        return
    end
    local thresholdPct = 100 - st.mult * st.modifier
    if postDamage * 100 >= thresholdPct * creature:GetMaxHealth() then
        return
    end
    local order = st.modifier - 1
    st.modifier = st.modifier + 1
    local b = ACTIVE_BOAT[st.order[order + 1] + 1]
    creature:InterruptNonMeleeSpells(true)
    creature:CastSpell(creature, SPELL_SCREAMS_OF_THE_DEAD)
    creature:AttackStop()
    creature:SetReactState(0) -- REACT_PASSIVE
    st.inBoat = true
    creature:MoveTo(POINT_BOAT, b.mx, b.my, b.mz)
    despawnBoatGhosts(st)
    if st.ancestorKey then
        cancelTimerKey(guid, st.ancestorKey)
        st.ancestorKey = nil
    end
    delayTimers(guid, { "bane", "fetidrot", "darkslash", "ancestorsvengeance" }, 10000)
    st.active = order + 1
end

-- C++ boss_ymironAI::MovementInform (:165-184): POINT_MOTION_TYPE +
-- POINT_BOAT gate; Talk(summon say) + SummonCreature(ancestor visual,
-- TEMPSUMMON_CORPSE_DESPAWN) + DoCast(ancestor, CHANNEL_YMIRON_TO_
-- SPIRIT 48307) + ancestor->CastSpell(me, CHANNEL_SPIRIT_TO_YMIRON
-- 48316, triggered) + SetFlag(NON_ATTACKABLE | NOT_SELECTABLE) +
-- SetDisableGravity(true); EVENT_RESUME_COMBAT in 5s. The IsInCombat +
-- inBoat guards drop stale arrivals (evade mid-spline: C++ evade
-- clears the motion, so no arrival fires).
local function ymironMovementInform(event, creature, moveType, id)
    if moveType ~= POINT_MOTION_TYPE or id ~= POINT_BOAT then
        return
    end
    local guid = creature:GetGUID()
    local st = boat[guid]
    if not st or not st.inBoat or not creature:IsInCombat() then
        return
    end
    st.inBoat = false
    local b = ACTIVE_BOAT[st.order[st.active] + 1]
    creature:Talk(b.say)
    local ancestor = creature:SummonCreature(b.npc, b.sx, b.sy, b.sz, b.so, 4)
    if ancestor then
        creature:CastSpell(ancestor, SPELL_CHANNEL_YMIRON_TO_SPIRIT)
        ancestor:CastSpell(creature, SPELL_CHANNEL_SPIRIT_TO_YMIRON, true)
        ancestor:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE)
        ancestor:SetDisableGravity(true)
        st.ancestor = ancestor
    end
    schedule(guid, "resume", 5000, function()
        resumeCombat(creature, guid, st)
    end)
end

-- C++ SelectTarget(Random, 0) (UnitAI.h:170): random over the threat
-- list, any unit, alive-filtered (the archavon randomThreatTarget
-- precedent — pets/totems eligible, no distance filter, no
-- player-only filter).
local function randomThreatTarget(creature)
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:IsAlive() then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ boss_ymironAI::JustSummoned (:186-206): NPC_SPIRIT_FOUNT ->
-- self-cast SPELL_SPIRIT_FOUNT 48380 (triggered) + SetDisplayId(11686)
-- + GUID latch; NPC_AVENGING_SPIRIT -> random-target AddThreat(0) +
-- AttackStart. Spell summons ride summonTempCreature, which fires
-- ev19 (lua_creature_events.go:190); the ancestor visuals fall
-- through the default case.
local function ymironJustSummoned(event, creature, summoned)
    local entry = summoned:GetEntry()
    if entry == NPC_SPIRIT_FOUNT then
        summoned:CastSpell(summoned, SPELL_SPIRIT_FOUNT, true)
        summoned:SetDisplayId(11686)
        local st = boat[creature:GetGUID()]
        if st then
            st.fount = summoned
        end
    elseif entry == NPC_AVENGING_SPIRIT then
        local target = randomThreatTarget(creature)
        if target then
            summoned:AddThreat(target, 0)
            summoned:AttackStart(target)
        end
    end
end

-- C++ boss_ymironAI::JustEngagedWith (:141-149): BossAI::JustEngagedWith
-- base-first (pulse + me-DoZone ported above Talk; instance/setActive
-- legs out); Talk(SAY_AGGRO) — unconditional (the auriaya engage-port
-- precedent); the four base-spell schedule inits are carried here on
-- the xt002 precedent.
local function ymironEnterCombat(event, creature, target)
    -- BossAI::JustEngagedWith (:143, base-first) — SetBossState(DATA_KING_YMIRON,
    -- IN_PROGRESS) PORTED tick 261 via creature:SetBossState
    -- (lua_creature_events.go:5917), first per BossAI::_JustEngagedWith
    -- (ScriptedCreature.cpp:537), before SetCombatPulseDelay(5)
    -- + me-form DoZoneInCombat (ScriptedCreature.cpp:540/542).
    creature:SetBossState(DATA_KING_YMIRON, 1)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    local heroic = creature:IsHeroic()
    cancelTimers(guid)
    schedule(guid, "bane", {18000, 23000}, function()
        baneTick(creature, guid)
    end)
    schedule(guid, "fetidrot", {8000, 13000}, function()
        fetidRotTick(creature, guid)
    end)
    schedule(guid, "darkslash", {28000, 33000}, function()
        darkSlashTick(creature, guid)
    end)
    schedule(guid, "ancestorsvengeance", heroic and 45000 or 60000, function()
        ancestorsVengeanceTick(creature, guid, heroic)
    end)
end

-- C++ KilledUnit (:274-278): if (who->GetTypeId() == TYPEID_PLAYER)
-- Talk(SAY_SLAY) — the razuvious player-gated variant precedent.
local function ymironKilledUnit(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied (:277-280): _JustDied() (SetBossState DONE,
-- ScriptedCreature.cpp:519) then Talk (:280) — the DONE leg PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917);
-- Talk(SAY_DEATH) — unconditional — the sjonnir JustDied-Talk precedent.
local function ymironJustDied(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ _JustDied() (ScriptedCreature.cpp:513-520): summons.DespawnAll();
    -- the boat state dies with the AI instance (ctor reshuffles on respawn).
    local st = boat[guid]
    if st then
        despawnBoatGhosts(st)
        boat[guid] = nil
    end
    creature:SetBossState(DATA_KING_YMIRON, 3) -- DONE
    creature:Talk(SAY_DEATH)
end

-- C++ boss_ymironAI::Reset (:134-141): _Reset() — SetBossState(DATA_KING_YMIRON 3,
-- NOT_STARTED) unless DONE (BossAI::_Reset, ScriptedCreature.cpp:509-510) PORTED
-- tick 261 via creature:SetBossState (lua_creature_events.go:5917), gated on
-- GetBossState (lua_creature_events.go:5936); the IsDead guard mirrors _Reset's
-- !IsAlive return (:501) and covers the ev23 death-path pre-fire (23 then 4);
-- then SetReactState(REACT_AGGRESSIVE) :140. Split from ev2 this tick (the
-- sjonnir precedent): ev2 leave-combat does not call Reset in C++, so it stays
-- cancel-only. Initialize() legs (ActivedNumber /
-- HealthAmountModifier / GUID clears) have no Lua mirrors — the kingsBane latch
-- is mirrored into SetData below (tick 417) and the boat-machine counters ride the
-- unbridged machine.
local function ymironReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:IsDead() then
        return
    end
    -- C++ Reset -> Initialize() (:122-130) + _Reset() summons.DespawnAll()
    -- (ScriptedCreature.cpp:507): modifier/active reset, boat ghosts
    -- despawned; ActiveOrder is NOT reshuffled (ctor-only).
    local st = boatStateFor(creature:GetGUID(), creature:IsHeroic())
    despawnBoatGhosts(st)
    st.modifier = 1
    st.active = 1
    st.ancestorKey = nil
    st.inBoat = false
    if creature:GetBossState(DATA_KING_YMIRON) ~= 3 then
        creature:SetBossState(DATA_KING_YMIRON, 0)
    end
    -- C++ Reset -> Initialize() (boss_ymiron.cpp:122-130): kingsBane =
    -- true. The IsDead guard above covers the ev23 death-path pre-fire
    -- (kill.go:1091); without it the re-arm would overwrite a
    -- Bane-cleared latch before the gate reads it.
    creature:SetData(DATA_KINGS_BANE, 1)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

-- C++ boss_ymironAI::SpellHitTarget (boss_ymiron.cpp:153-157): if the
-- BANE_HIT 59203 hit lands on a player, kingsBane = false. Event 15
-- fires on the caster when its spell hits a unit (boss_ai.go:4011);
-- the target player gate is the nalorakk GetObjectType() variant.
local function ymironSpellHitTarget(event, creature, target, spellId)
    if spellId ~= SPELL_BANE_HIT then
        return
    end
    if target and target:GetObjectType() == "Player" then
        creature:SetData(DATA_KINGS_BANE, 0)
    end
end

RegisterCreatureEvent(ENTRY_KING_YMIRON, 1, ymironEnterCombat)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 2, cancelCombatTimers)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 3, ymironKilledUnit)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 4, ymironJustDied)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 9, ymironDamageTaken)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 15, ymironSpellHitTarget)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 19, ymironJustSummoned)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 23, ymironReset)
RegisterCreatureEvent(ENTRY_KING_YMIRON, 28, ymironMovementInform)
