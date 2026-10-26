-- Arlokk (Zul'Gurub) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulGurub/boss_arlokk.cpp
-- (boss_arlokkAI + npc_zulian_prowlerAI + go_gong_of_bethekkAI);
-- zulgurub.h:33 (DATA_ARLOKK = 3), :49 (NPC_ARLOKK = 14515),
-- :51 (NPC_ZULIAN_PROWLER = 15101), :70 (GO_GONG_OF_BETHEKK = 180526).
-- Creature entries: 14515 Arlokk (C++ ScriptName "boss_arlokk" per
-- AddSC_boss_arlokk); 15101 Zulian prowler (C++ ScriptName
-- "npc_zulian_prowler"). The panther trigger 15091 has no C++ script
-- and is not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Fight shape (C++ PHASE_ONE=1 troll / PHASE_TWO=2 panther):
--   troll: Talk SAY_AGGRO(0); shadow word pain 24212 at {7s,9s} then
--     {5s,7s} on the victim (triggered); gouge 12540 at {12s,15s} on
--     the victim (triggered, one-shot — C++ does not re-arm it);
--     mark of arlokk 24210 at {9s,11s} then {120s,130s} on a random
--     alive player (C++: random of 2nd-4th threat not marked; no threat
--     model) with victim fallback, triggered cast, then Talk
--     SAY_FEAST_PROWLER(1); panther transform at {15s,20s} (one-shot).
--   transform: non-triggered self-cast 24190 (panther transform);
--     unequip/weapon/AttackStop/threat/passive/flag arms PORTED
--     (see onTransform); self-cast 24222 (vanish visual) then 24223 (vanish);
--     1s later self-cast 24235 (super invis), move to a random spot
--     (no movement model — the boss just stays invisible);
--     9s later re-cast 24223 + 24235; {7s,10s} later re-appear:
--     RemoveAura(24235), RemoveAura(24223), arm ravage 24213 at
--     {10s,14s} (triggered on victim) and transform back at
--     {15s,18s}, phase -> panther.
--   transform back: RemoveAura(24190); self-cast 24222 vanish visual;
--     re-arm shadow word pain {4s,7s}, gouge {12s,15s}, transform
--     {16s,20s} (all PHASE_ONE), phase -> troll.
--   died: Talk SAY_DEATH(2), cancel timers.
-- Prowler (15101): OnReset self-casts the sneak auras 22766 + 7939
-- (C++-exact non-triggered casts; the MovePoint to Arlokk has no
-- bridge); OnEnterCombat removes both sneak auras (C++
-- JustEngagedWith). Melee is engine-driven. The 6s attack timer has
-- no Attack bridge; the SpellHit(24211) arm stays out on the same
-- blocker — stale "no SpellHit bridge" retracted (2026-10-21; ev14
-- fires), but the arm's whole body is me->Attack(unitCaster, true)
-- (boss_arlokk.cpp:378-385), which has no bridge (registering it
-- would be a stub, firesworn convention); the JustDied
-- arlokk-SetData(_sideData, 0) leg is cross-creature summon
-- bookkeeping with no bridge (stays out — the 4s despawn leg IS
-- ported below).
-- Deviations from C++: no prowler-summon model — the zulian prowlers
-- and the panther triggers 15091 never exist, so EVENT_SUMMON_PROWLERS
-- (list-enumeration + spell-summon legs, see arlokkEnterCombat) and the
-- SetData count arms are skipped; the go_gong_of_bethekk script IS
-- ported (gong flag/anim/summon, see onGongGossip); no
-- instance-script model — boss admission via the luaBossAI shim, and
-- the EnterEvadeMode gong-flag arm is skipped (no EnterEvadeMode bridge;
-- instance GO lookup unbridged); no threat model — mark-of-arlokk target is a random alive
-- player (victim fallback); the vanish ResetThreatList arm IS
-- ported (:285 — ResetAllThreat bridged); equipment PORTED
-- via creature:SetEquipmentSlots (lua_creature_events.go:451, 2026-10-22):
-- the Reset dagger restore + EVENT_TRANSFORM unequip +
-- EVENT_TRANSFORM_BACK re-equip arms; flag
-- arms PORTED (see onTransform / onVanish2); the REACT_PASSIVE arm is PORTED
-- (creature:SetReactState, 2026-10-20); the C++ damage +/-35% hack
-- is PORTED (ApplyDamagePctModifier, 2026-10-26); no movement model —
-- the Reset MovePoint, vanish MovePoint, and AttackStart arms skipped;
-- Talk(id, target) is bridged (the whisperTarget arm, engine/world/
-- creature_text.go:62 — C++-exact; non-whisper rows ignore the
-- target, whisper rows route to that player only) — SAY_FEAST_PROWLER
-- passes the mark target (boss_arlokk.cpp:227).

local ENTRY_ARLOKK = 14515
local ENTRY_PROWLER = 15101
local GO_GONG_OF_BETHEKK = 180526
local GAMEOBJECT_FLAGS = 9 -- UpdateFields.h: OBJECT_END 6 + 3 (Go gameObjectFlags twin)
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local POS_SUMMON_ARLOKK = {-11507.22, -1628.062, 41.38264, 3.159046} -- C++ PosSummonArlokk[0]

local SAY_AGGRO = 0
local SAY_FEAST_PROWLER = 1
local SAY_DEATH = 2

local SPELL_SHADOW_WORD_PAIN = 24212
local SPELL_GOUGE = 12540
local SPELL_MARK_OF_ARLOKK = 24210
local SPELL_RAVAGE = 24213
local SPELL_PANTHER_TRANSFORM = 24190
local SPELL_VANISH_VISUAL = 24222
local WEAPON_DAGGER = 10616 -- boss_arlokk.cpp:72 (dagger item id)
local SPELL_VANISH = 24223
local SPELL_SUPER_INVIS = 24235
local SPELL_SNEAK_1 = 22766
local SPELL_SNEAK_2 = 7939

local PHASE_TROLL = 1
local PHASE_PANTHER = 2

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function arlokkState(guid)
    local st = state[guid]
    if not st then
        st = { panther = false }
        state[guid] = st
    end
    return st
end

-- C++ EVENT_MARK_OF_ARLOKK (boss_arlokk.cpp:221): MaxThreat with a
-- random 1-3 offset over the victim-first list (UnitAI.h: victim,
-- then the rest in threat order; the offset pop runs before
-- filtering, so the victim never wins the pick), any unit
-- (playerOnly=false), excluding units already carrying the mark
-- (aura -24210); the first surviving entry wins, not a random draw.
local function markTarget(creature)
    local targets = creature:GetAITargets()
    local victim = creature:GetVictim()
    local victimGuid = victim ~= nil and victim:GetGUID() or nil
    local ordered = {}
    if victim ~= nil then
        ordered[#ordered + 1] = victim
    end
    for _, unit in ipairs(targets) do
        if unit:GetGUID() ~= victimGuid then
            ordered[#ordered + 1] = unit
        end
    end
    local skip = math.random(1, 3)
    for i, unit in ipairs(ordered) do
        if i > skip and not unit:IsDead()
                and not unit:HasAura(SPELL_MARK_OF_ARLOKK) then
            return unit
        end
    end
    return nil
end

-- C++ shadow word pain (troll): triggered victim cast, re-arm {5s,7s}
-- while still in troll form (phase gate collapses to a state check).
local function onShadowWordPain(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOW_WORD_PAIN, true)
    end
    schedule(timers, guid, "swp", math.random(5000, 7000), function()
        onShadowWordPain(creature, guid)
    end)
end

-- C++ gouge (troll): triggered victim cast, one-shot (C++ does not
-- re-arm EVENT_GOUGE).
local function onGouge(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_GOUGE, true)
    end
end

-- C++ mark of arlokk (both phases): triggered cast on the
-- MaxThreat-offset pick (victim fallback), then Talk(1); re-arm
-- {120s,130s}.
local function onMarkOfArlokk(creature, guid)
    local target = markTarget(creature) or creature:GetVictim()
    if target then
        creature:CastSpell(target, SPELL_MARK_OF_ARLOKK, true)
        creature:Talk(SAY_FEAST_PROWLER, target)
    end
    schedule(timers, guid, "mark", math.random(120000, 130000), function()
        onMarkOfArlokk(creature, guid)
    end)
end

-- C++ EventMap phase semantics (src/common/Utilities/EventMap.cpp
-- ExecuteEvent): while no phase is set (_phase == 0 — the whole troll
-- stretch up to EVENT_VISIBLE), every due event fires regardless of
-- the phase it was scheduled with. So the transform does NOT interrupt
-- the troll timers: shadow word pain keeps firing through the vanish
-- chain. SetPhase(PHASE_TWO) at EVENT_VISIBLE lazily discards the
-- pending phase-one events; SetPhase(PHASE_ONE) at transform back
-- discards the pending phase-two events. The cancelKeys calls below
-- model exactly those two discards.
local function cancelKeys(store, guid, keys)
    local per = store[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

-- Forward declarations: onVanish2 schedules onRavage/onTransformBack,
-- which are defined below (halazzi/zuljin convention).
local onRavage
local onTransformBack

-- C++ panther transform: self-cast 24190 (non-triggered), vanish
-- visual 24222, vanish 24223; ResetThreatList PORTED 2026-10-19 via
-- creature:ResetAllThreat (bridged lua_creature_events.go); AttackStop /
-- SetFlag / REACT_PASSIVE arms PORTED 2026-10-22 (equipment PORTED via
-- creature:SetEquipmentSlots); the movement legs still have no bridge. The C++
-- EVENT_TRANSFORM was scheduled with
-- phase PHASE_ONE, so the 1s vanish chain only runs in troll form.
local function onVanish2(creature, guid)
    local st = arlokkState(guid)
    if st.panther then
        return
    end
    creature:CastSpell(creature, SPELL_VANISH)
    creature:CastSpell(creature, SPELL_SUPER_INVIS)
    schedule(timers, guid, "visible", math.random(7000, 10000), function()
        local st2 = arlokkState(guid)
        if st2.panther then
            return
        end
        -- C++ boss_arlokk.cpp:264: me->SetReactState(REACT_AGGRESSIVE)
        -- (EVENT_VISIBLE: flip is the first arm).
        creature:SetReactState(2) -- REACT_AGGRESSIVE
        -- C++ boss_arlokk.cpp:267-268: me->RemoveFlag(UNIT_FIELD_FLAGS,
        -- UNIT_FLAG_NON_ATTACKABLE|UNIT_FLAG_NOT_SELECTABLE) — the ported
        -- EVENT_TRANSFORM SetFlag pair's removal; AttackStart rides the
        -- unbridged attack machine.
        creature:RemoveFlag(59, 0x02000002)
        -- C++ EVENT_VISIBLE: SetPhase(PHASE_TWO) discards the pending
        -- phase-one timers (shadow word pain / gouge / transform).
        cancelKeys(timers, guid, { "swp", "gouge", "transform" })
        st2.panther = true
        -- C++ EVENT_VISIBLE (boss_arlokk.cpp:273): panther form +35%
        -- weapon damage via ApplyStatPctModifier (TOTAL_PCT).
        creature:ApplyDamagePctModifier(35)
        creature:RemoveAura(SPELL_SUPER_INVIS)
        creature:RemoveAura(SPELL_VANISH)
        schedule(timers, guid, "ravage", math.random(10000, 14000),
            function()
                onRavage(creature, guid)
            end)
        schedule(timers, guid, "transformback", math.random(15000, 18000),
            function()
                onTransformBack(creature, guid)
            end)
    end)
end

local function onVanish(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    creature:CastSpell(creature, SPELL_SUPER_INVIS)
    schedule(timers, guid, "vanish2", 9000, function()
        onVanish2(creature, guid)
    end)
end

local function onTransform(creature, guid)
    if arlokkState(guid).panther then
        return
    end
    -- C++ does not cancel the troll timers here: with _phase still 0
    -- they keep firing through the vanish chain (see cancelKeys note).
    creature:CastSpell(creature, SPELL_PANTHER_TRANSFORM)
    -- C++ boss_arlokk.cpp:235-236: me->SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID
    -- + 0/1, EQUIP_UNEQUIP) — unequip for the panther form (equipment bridged
    -- 2026-10-22 via creature:SetEquipmentSlots, lua_creature_events.go:451;
    -- ranged slot skipped, C++ never writes it).
    creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
    -- C++ boss_arlokk.cpp:241-245 order: AttackStop -> ResetThreatList ->
    -- REACT_PASSIVE -> SetFlag(59, NON_ATTACKABLE|NOT_SELECTABLE).
    creature:AttackStop()
    -- C++ EVENT_TRANSFORM: ResetThreatList() between the unequip and the
    -- vanish casts (bridged lua_creature_events.go); the movement arms
    -- still have no bridge (the REACT_PASSIVE arm is PORTED 2026-10-20 —
    -- creature:SetReactState; equipment PORTED above; AttackStop/flag
    -- PORTED 2026-10-22 — bridged lua_creature_events.go:180/:968).
    creature:ResetAllThreat()
    creature:SetReactState(0) -- REACT_PASSIVE
    creature:SetFlag(59, 0x02000002) -- UNIT_FLAG_NON_ATTACKABLE|UNIT_FLAG_NOT_SELECTABLE
    creature:CastSpell(creature, SPELL_VANISH_VISUAL)
    creature:CastSpell(creature, SPELL_VANISH)
    schedule(timers, guid, "vanish", 1000, function()
        onVanish(creature, guid)
    end)
end

-- C++ ravage (panther): triggered victim cast, re-arm {10s,14s} while
-- still in panther form.
onRavage = function(creature, guid)
    if not arlokkState(guid).panther then
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_RAVAGE, true)
    end
    schedule(timers, guid, "ravage", math.random(10000, 14000), function()
        onRavage(creature, guid)
    end)
end

-- C++ transform back: RemoveAura(24190), vanish visual; the weapon
-- damage-reset arm is PORTED below (inverse of +35%).
-- Troll timers are re-
-- armed fresh; the panther timers are dropped.
onTransformBack = function(creature, guid)
    local st = arlokkState(guid)
    if not st.panther then
        return
    end
    st.panther = false
    -- C++ transform back: SetPhase(PHASE_ONE) discards the pending
    -- phase-two timers (ravage / transform back); the troll timers are
    -- re-armed fresh below.
    cancelKeys(timers, guid, { "ravage", "transformback" })
    creature:RemoveAura(SPELL_PANTHER_TRANSFORM)
    -- C++ EVENT_TRANSFORM_BACK (boss_arlokk.cpp:291): the rollback is
    -- the inverse of +35% (100/1.35-100), restoring the group to 1.0.
    creature:ApplyDamagePctModifier(100 / 1.35 - 100)
    creature:CastSpell(creature, SPELL_VANISH_VISUAL)
    -- C++ boss_arlokk.cpp:283-284: UNIT_VIRTUAL_ITEM_SLOT_ID + 0/1 =
    -- WEAPON_DAGGER — re-equip for troll form (bridged via
    -- creature:SetEquipmentSlots; ranged slot skipped, C++ never writes it).
    creature:SetEquipmentSlots(WEAPON_DAGGER, WEAPON_DAGGER, 0xFFFFFFFF)
    schedule(timers, guid, "swp", math.random(4000, 7000), function()
        onShadowWordPain(creature, guid)
    end)
    schedule(timers, guid, "gouge", math.random(12000, 15000), function()
        onGouge(creature, guid)
    end)
    schedule(timers, guid, "transform", math.random(16000, 20000), function()
        onTransform(creature, guid)
    end)
end

local function arlokkResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { panther = false }
end

-- C++ JustEngagedWith: Talk SAY_AGGRO; arm shadow word pain, gouge,
-- mark of arlokk, transform. The panther-trigger scan and prowler
-- summon stay out — the arm needs the full trigger LIST (GetCreatureListWithEntryInGrid,
-- 100.0f, split by Y < -1625 into 5-per-side GUID arrays) and a
-- trigger CastSpell(SPELL_SUMMON_PROWLER) delivery (no spell-summon
-- footprint; single FindNearestCreature + direct SummonCreature cannot
-- express either leg) — the 6s prowler timer is not scheduled.
local function arlokkEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    arlokkResetState(guid)
    creature:Talk(SAY_AGGRO)
    schedule(timers, guid, "swp", math.random(7000, 9000), function()
        onShadowWordPain(creature, guid)
    end)
    schedule(timers, guid, "gouge", math.random(12000, 15000), function()
        onGouge(creature, guid)
    end)
    schedule(timers, guid, "mark", math.random(9000, 11000), function()
        onMarkOfArlokk(creature, guid)
    end)
    schedule(timers, guid, "transform", math.random(15000, 20000), function()
        onTransform(creature, guid)
    end)
end

local function arlokkLeaveCombat(event, creature)
    arlokkResetState(creature:GetGUID())
end

-- C++ JustDied: _JustDied (instance, no bridge), Talk SAY_DEATH.
local function arlokkDied(event, creature, killer)
    arlokkResetState(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

-- C++ Reset (boss_arlokk.cpp:112-116): IsInPhase(PHASE_TWO) damage
-- rollback first, then UNIT_VIRTUAL_ITEM_SLOT_ID + 0/1 = WEAPON_DAGGER
-- (SetWalk/MovePoint legs stay out: no movement bridge). The alive
-- gate restores C++ Reset-never-runs-on-death: Go fires 23 before 4
-- on death, so an ungated rollback would leak into the next life.
local function arlokkReset(event, creature)
    local wasPanther = arlokkState(creature:GetGUID()).panther
    arlokkResetState(creature:GetGUID())
    if wasPanther and creature:IsAlive() then
        creature:ApplyDamagePctModifier(100 / 1.35 - 100)
    end
    creature:SetEquipmentSlots(WEAPON_DAGGER, WEAPON_DAGGER, 0xFFFFFFFF)
end

RegisterCreatureEvent(ENTRY_ARLOKK, 1, arlokkEnterCombat)
RegisterCreatureEvent(ENTRY_ARLOKK, 2, arlokkLeaveCombat)
RegisterCreatureEvent(ENTRY_ARLOKK, 4, arlokkDied)
RegisterCreatureEvent(ENTRY_ARLOKK, 23, arlokkReset)

-- npc_zulian_prowler (15101, C++ ScriptName "npc_zulian_prowler"):
-- OnReset self-casts sneak auras 22766 + 7939; OnEnterCombat removes
-- both (C++ JustEngagedWith); melee is engine-driven. The MovePoint to
-- Arlokk, the 6s random-attack timer (no Attack bridge) and the
-- SpellHit(24211) attack arm have no bridge and are not modeled. The
-- JustDied arlokk-SetData(_sideData, 0) leg is cross-creature summon
-- bookkeeping with no bridge (stays out); the JustDied 4s
-- DespawnOrUnsummon leg is PORTED below (prowlerDied) as a one-shot
-- CreateLuaEvent (the stale "4s despawn has no bridge" claim is
-- retracted — creature:Despawn() is bridged,
-- engine/world/lua_creature_events.go:370).
local function prowlerEnterCombat(event, creature, target)
    creature:RemoveAura(SPELL_SNEAK_1)
    creature:RemoveAura(SPELL_SNEAK_2)
end

-- C++ Reset (boss_arlokk.cpp:353-370): the sneak casts are spawn/evade-only,
-- never run on death; they do not belong on the corpse.
local function prowlerReset(event, creature)
    cancelTimers(timers, creature:GetGUID())
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_SNEAK_1)
        creature:CastSpell(creature, SPELL_SNEAK_2)
    end
end

-- C++ npc_zulian_prowlerAI::JustDied (boss_arlokk.cpp:388-396): the
-- arlokk-SetData(_sideData, 0) leg has no cross-creature bridge (stays
-- out — summon bookkeeping); the unconditional 4s DespawnOrUnsummon
-- ports C++-exact as a one-shot CreateLuaEvent. The timer rides the
-- file's timers idiom and is cancelled on 2/4/23 (prowlerReset covers
-- 23; a dead prowler cannot leave combat, so 2 never fires here).
local function prowlerDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(timers, guid)
    schedule(timers, guid, "despawn", 4000, function()
        creature:Despawn()
        local per = timers[guid]
        if per then
            per["despawn"] = nil
        end
    end)
end

RegisterCreatureEvent(ENTRY_PROWLER, 1, prowlerEnterCombat)
RegisterCreatureEvent(ENTRY_PROWLER, 4, prowlerDied)
RegisterCreatureEvent(ENTRY_PROWLER, 23, prowlerReset)

-- go_gong_of_bethekk (180526, C++ ScriptName "go_gong_of_bethekk",
-- boss_arlokk.cpp:452-458): OnGossipHello PORTED — SetFlag via the new
-- go:SetFlag bridge (lua_objects.go), SendCustomAnim via go:SendCustomAnim,
-- summon via player:SummonCreature (the GO-summoner twin: the GO AI has
-- no JustSummoned arm, so the player summoner's no-event-19 fire matches
-- C++; type 3 TIMED_DESPAWN_OUT_OF_COMBAT, 10min). C++ returns true
-- (handled) == Lua true (brazier polarity). The EnterEvadeMode gong-flag
-- arm stays out (no EnterEvadeMode bridge; instance GO lookup unbridged).
local function onGongGossip(_, player, go)
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    go:SendCustomAnim(0)
    player:SummonCreature(ENTRY_ARLOKK, POS_SUMMON_ARLOKK[1], POS_SUMMON_ARLOKK[2], POS_SUMMON_ARLOKK[3], POS_SUMMON_ARLOKK[4], 3, 600000)
    return true
end

RegisterGameObjectGossipEvent(GO_GONG_OF_BETHEKK, 1, onGongGossip)
