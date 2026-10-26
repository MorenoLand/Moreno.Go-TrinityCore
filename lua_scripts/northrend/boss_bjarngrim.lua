-- General Bjarngrim (Halls of Lightning) — Lua port of
-- src/server/scripts/Northrend/Ulduar/HallsOfLightning/boss_bjarngrim.cpp
-- (boss_bjarngrim (CreatureScript) via GetHallsOfLightningAI<boss_bjarngrimAI>
-- (ScriptedAI); npc_stormforged_lieutenant (CreatureScript) via
-- GetHallsOfLightningAI<npc_stormforged_lieutenantAI> (ScriptedAI); both
-- registered from inside AddSC_boss_bjarngrim(); loader decl 98 /
-- call 293 per northrend_script_loader.cpp — the FIRST group of the
-- "// Halls of Lightning" block in AddNorthrendScripts(), immediately
-- after AddSC_instance_obsidian_sanctum() (decl 96 / call 291), under
-- the "// Halls of Lightning" marker at loader line 292; the call after
-- it is AddSC_boss_loken() (decl 99 / call 294) — loader order
-- confirmed this run; the checkpoint sequence
-- (instance_obsidian_sanctum -> boss_bjarngrim) is followed).
-- Entry: 28586 General Bjarngrim (halls_of_lightning.h NPC_BJARNGRIM,
-- line 39; DATA_BJARNGRIM = 0, line 31);
-- instance_halls_of_lightning.cpp OnCreatureCreate binds case
-- NPC_BJARNGRIM (line 51) — entry-verifiable, registration proceeds
-- (the nexus_commanders / malygos / sartharion kalecgos precedent); the
-- CreatureScript ScriptName binding is DB-side as usual.
-- Sole-source verified: whole-server-tree grep for "boss_bjarngrim"
-- and "npc_stormforged_lieutenant" hits boss_bjarngrim.cpp (+ the loader
-- decl/call lines for AddSC_boss_bjarngrim) only; zero sql/ hits. No
-- bjarngrim lua existed.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied
-- (bjarngrim); 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 23 OnReset
-- (lieutenant). Timer arms ride the CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23; CastSpell(nil,
-- spell) = DoCastVictim; all casts here are 2-arg / non-triggered
-- (boss_bjarngrim.cpp); {min,max} delay tables = urand re-arms. Melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith — Talk(SAY_AGGRO 0) + CallForHelp(30.0f) emulation
-- (event 1): lieutenants in 30y (GetCreaturesInGrid 29240) with no
-- victim engage the entering player (C++ CallOfHelpCreatureInRangeDo
-- EngageWithTarget leg; IsEngaged skip via !GetVictim, LOS/faction/
-- react gates unmodeled, 2D+reach scan vs C++ 3D absolute 30.0f).
-- instance->SetBossState(DATA_BJARNGRIM, IN_PROGRESS) has no bridge
-- — the boss_bookkeeping precedent.
-- KilledUnit — Talk(SAY_SLAY 4) (event 3 — C++-UNCONDITIONAL: the Talk
-- fires for any victim; no nalorakk gate here — the anubrekhan
-- unconditional variant precedent).
-- JustDied — Talk(SAY_DEATH 5) (event 4;
-- instance->SetBossState(DATA_BJARNGRIM, DONE) has no instance bridge).
-- DOCUMENTED-ONLY (in this header; no registration beyond entry 28586
-- — no bridgeable arms):
-- PORTED: npc_stormforged_lieutenant (NPC_STORMFORGED_LIEUTENANT =
-- 29240, local enum in boss_bjarngrim.cpp — entry-verifiable from the
-- local enum, the acolyte_of_shadron / acolyte_of_vesperon precedent) —
-- UpdateAI SPELL_ARC_WELD 59085: DoCastVictim non-triggered,
-- urand(20000,21000) init / re-arm (:409/:440-441), zero gating legs —
-- ported via CreateLuaEvent on OnEnterCombat (events 2/4/23 cancel).
-- The lieutenant's remaining arms are DOCUMENTED-ONLY (in this header):
-- Reset (Initialize timer clears — folds into combat registration);
-- JustEngagedWith (:424-432 — instance->GetGuidData(DATA_BJARNGRIM) +
-- IsAlive / GetVictim gate + AttackStart — no instance bridge);
-- UpdateAI DoCast(pBjarngrim, SPELL_RENEW_STEEL_N 52774) (:451,
-- urand(10000,11000) init / urand(10000,14000) re-arm — gated on
-- instance->GetGuidData(DATA_BJARNGRIM) + IsAlive (:446-449) — no
-- instance bridge; DOCUMENTED DEVIATION: in C++ this fires on a live
-- bjarngrim, in Lua it never fires). Lieutenant has no Talk arms
-- (:380-460 confirmed — no Talk( in the class).
-- boss_bjarngrim Reset / Initialize — PORTED 2026-10-18 (the
-- canBuff AddAura(SPELL_TEMPORARY_ELECTRICAL_CHARGE 52092) leg,
-- boss_bjarngrim.cpp:163-167: re-buffs on Reset only if canBuff is true
-- and the aura is absent); the m_auiStormforgedLieutenantGUID respawn
-- loop (C++ self-notes the GUIDs are never assigned) and
-- instance->SetBossState(DATA_BJARNGRIM, NOT_STARTED) stay out — no
-- summon / instance bridges. The stance-reset DoRemoveStanceAura +
-- DoCast(SPELL_DEFENSIVE_STANCE 53790) leg and the SetEquipmentSlots
-- restore (EQUIP_SWORD 37871 / EQUIP_SHIELD 35642) are PORTED 2026-10-25
-- on ev23 (inside bjarngrimReset below). (The stale "no equipment bridge"
-- claim is retracted: creature:SetEquipmentSlots is bridged on the event
-- surface, and the stance-machine equipment arms (:268/:274/:279) are
-- PORTED with the stance machine below.)
-- EnterEvadeMode — PORTED 2026-10-18 (canBuff = HasAura(52092)
-- snapshot, boss_bjarngrim.cpp:191-196 — no re-buff after dispel).
-- DoRemoveStanceAura (stance-switched RemoveAurasDueToSpell 53790 /
-- 53791 / 53792) — PORTED 2026-10-25 as part of the stance machine
-- below (removal on every stance change + on ev23 stance reset;
-- creature:RemoveAura is bridged, lua_creature_events.go:153).
-- UpdateAI — the stance-change machine (:123/:284, urand(20000,25000)
-- init/re-arm) — PORTED 2026-10-25 (bjarngrimStancePump below): the
-- stance Talk/Emote arms (defensive 1/6 :265-266, berserker 3/8
-- :271-272, battle 2/7 :277-278) + stance casts (:267/:273/:279) +
-- equipment swaps (:269/:274/:279) + the nine stance-gated spell
-- timers (REFLECTION 36096 :296-297, KNOCK_AWAY 52029 :304-305,
-- PUMMEL 12555 :312-313, IRONFORM 52022 :320-321, INTERCEPT 58769
-- :333-334, WHIRLWIND 52027 :341-342, CLEAVE 15284 :349-350,
-- MORTAL_STRIKE 16856 :361-362, SLAM 52026 :369-370). The 2026-10-14
-- STAY-OUT rationale is retired: both cited blockers were stale
-- (RemoveAura + SetEquipmentSlots are bridged) and the machine is pure
-- Lua state. Per-stance timers freeze while another stance is active
-- (C++ switch branches :283-375), modeled by per-stance remaining
-- times decremented only for the active stance on a 500ms pump tick;
-- a change tick skips the stance timers, matching the C++ return
-- (:283). DELTA: the IsNonMeleeSpellCast wait (:254) has no Lua
-- bridge, so a change landing mid-cast is not deferred by a tick.

local ENTRY_BJARNGRIM = 28586
local ENTRY_STORMFORGED_LIEUTENANT = 29240

local SAY_AGGRO = 0
local SAY_SLAY = 4
local SAY_DEATH = 5

local SPELL_ARC_WELD = 59085

-- C++ boss_bjarngrimAI stance machine (boss_bjarngrim.cpp:255-375),
-- PORTED 2026-10-25. m_uiStance starts DEFENSIVE; the change timer
-- (urand(20000,25000) init/re-arm, :161/:284) picks a DIFFERENT stance
-- uniformly (rand32() % 2, bumped past the current one, :255-259). On
-- change the old stance aura is removed, SAY + EMOTE Talks fire, the
-- new stance spell is self-cast, and equipment swaps (sword+shield /
-- sword+sword / mace+unequip). Per-stance timers tick only while their
-- stance is active (C++ switch branches :283-375), so the pump below
-- keeps per-stance remaining times and decrements only the active
-- stance's timers each 500ms tick; a change tick skips the stance
-- timers, matching the C++ return (:283). DELTA: the IsNonMeleeSpellCast
-- wait (:254) has no Lua bridge, so a change landing mid-cast is not
-- deferred by a tick. All casts are 2-arg / non-triggered.
local STANCE_DEFENSIVE = 0
local STANCE_BERSERKER = 1
local STANCE_BATTLE = 2

local STANCE_SPELL = { [0] = 53790, [1] = 53791, [2] = 53792 }
local STANCE_SAY = { [0] = 1, [1] = 3, [2] = 2 }
local STANCE_EMOTE = { [0] = 6, [1] = 8, [2] = 7 }
local STANCE_EQUIP = { [0] = { 37871, 35642 }, [1] = { 37871, 37871 }, [2] = { 43623, 0 } }

local STANCE_TIMERS = {
    [0] = {
        { key = "reflection", spell = 36096, victim = false, init = 8000, min = 8000, max = 9000 },
        { key = "knockaway", spell = 52029, victim = false, init = 20000, min = 20000, max = 21000 },
        { key = "pummel", spell = 12555, victim = true, init = 10000, min = 10000, max = 11000 },
        { key = "ironform", spell = 52022, victim = false, init = 25000, min = 25000, max = 26000 },
    },
    [1] = {
        { key = "intercept", spell = 58769, victim = true, init = 5000, min = 45000, max = 46000 },
        { key = "whirlwind", spell = 52027, victim = false, init = 10000, min = 10000, max = 11000 },
        { key = "cleave", spell = 15284, victim = true, init = 8000, min = 8000, max = 9000 },
    },
    [2] = {
        { key = "mortal", spell = 16856, victim = true, init = 8000, min = 20000, max = 21000 },
        { key = "slam", spell = 52026, victim = true, init = 10000, min = 15000, max = 16000 },
    },
}

local bjarngrimStance = {}
local bjarngrimStancePumpIds = {}

local function bjarngrimStanceSchedule(guid, delay, fn)
    local old = bjarngrimStancePumpIds[guid]
    if old then
        RemoveEventById(old)
    end
    bjarngrimStancePumpIds[guid] = CreateLuaEvent(fn, delay)
end

local function bjarngrimStanceCancel(guid)
    local id = bjarngrimStancePumpIds[guid]
    if id then
        RemoveEventById(id)
        bjarngrimStancePumpIds[guid] = nil
    end
end

local function bjarngrimStanceInit(guid)
    local remaining = {}
    for _, specs in pairs(STANCE_TIMERS) do
        for _, spec in ipairs(specs) do
            remaining[spec.key] = spec.init
        end
    end
    bjarngrimStance[guid] = { stance = STANCE_DEFENSIVE, change = math.random(20000, 25000), remaining = remaining }
end

local function bjarngrimStancePump(creature, guid)
    local st = bjarngrimStance[guid]
    if not st then
        return
    end
    st.change = st.change - 500
    if st.change <= 0 then
        local old = st.stance
        creature:RemoveAura(STANCE_SPELL[old])
        local others = {}
        for s = 0, 2 do
            if s ~= old then
                others[#others + 1] = s
            end
        end
        local newStance = others[math.random(2)]
        st.stance = newStance
        creature:Talk(STANCE_SAY[newStance])
        creature:Talk(STANCE_EMOTE[newStance])
        creature:CastSpell(creature, STANCE_SPELL[newStance])
        local equip = STANCE_EQUIP[newStance]
        creature:SetEquipmentSlots(equip[1], equip[2], 0xFFFFFFFF)
        st.change = math.random(20000, 25000)
    else
        for _, spec in ipairs(STANCE_TIMERS[st.stance]) do
            local left = st.remaining[spec.key] - 500
            if left <= 0 then
                if spec.victim then
                    creature:CastSpell(nil, spec.spell)
                else
                    creature:CastSpell(creature, spec.spell)
                end
                left = math.random(spec.min, spec.max)
            end
            st.remaining[spec.key] = left
        end
    end
    bjarngrimStanceSchedule(guid, 500, function()
        bjarngrimStancePump(creature, guid)
    end)
end

local function bjarngrimStanceStart(creature)
    local guid = creature:GetGUID()
    bjarngrimStanceInit(guid)
    bjarngrimStanceSchedule(guid, 500, function()
        bjarngrimStancePump(creature, guid)
    end)
end

-- C++ Reset (:176-186): if stance != DEFENSIVE, remove the stance aura,
-- cast DEFENSIVE_STANCE, restore sword+shield, re-init timers. Runs on
-- ev23 (spawn / evade / respawn); at spawn the state is fresh-defensive
-- so no cast fires, matching the C++ guard. The pump itself is canceled
-- on ev2/ev4 and rebuilt on the next ev1.
local function bjarngrimStanceReset(creature)
    local guid = creature:GetGUID()
    local st = bjarngrimStance[guid]
    if st and st.stance ~= STANCE_DEFENSIVE then
        creature:RemoveAura(STANCE_SPELL[st.stance])
        creature:CastSpell(creature, STANCE_SPELL[STANCE_DEFENSIVE])
        local equip = STANCE_EQUIP[STANCE_DEFENSIVE]
        creature:SetEquipmentSlots(equip[1], equip[2], 0xFFFFFFFF)
        st.stance = STANCE_DEFENSIVE
        for _, specs in pairs(STANCE_TIMERS) do
            for _, spec in ipairs(specs) do
                st.remaining[spec.key] = spec.init
            end
        end
        st.change = math.random(20000, 25000)
    end
end

-- C++ JustEngagedWith: Talk(AGGRO) + CallForHelp(30.0f) +
-- SetBossState(DATA_BJARNGRIM, IN_PROGRESS) — only the Talk arm is
-- bridgeable. C++ UpdateAI timers run only in combat and Reset
-- re-inits them, so ev1 (re)builds the whole stance state.
local function bjarngrimEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    if target then
        for _, lt in ipairs(creature:GetCreaturesInGrid(ENTRY_STORMFORGED_LIEUTENANT, 30)) do
            if not lt:GetVictim() then
                lt:EngageWithTarget(target)
            end
        end
    end
    bjarngrimStanceStart(creature)
end

-- C++ KilledUnit: unconditional Talk(SAY_SLAY) — the anubrekhan
-- unconditional variant precedent: the Talk fires for any victim.
local function bjarngrimTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied: Talk(DEATH) + SetBossState(DATA_BJARNGRIM, DONE) —
-- only the Talk arm is bridgeable.
local function bjarngrimDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    bjarngrimStanceCancel(creature:GetGUID())
end

-- CreateLuaEvent timer harness (GUID-keyed timers table, cancel on
-- events 2/4/23) — the maiden / thorim-colossus convention.
local timers = {}

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
    per[key] = CreateLuaEvent(fn, delay)
end

-- C++ npc_stormforged_lieutenantAI::UpdateAI: DoCastVictim(SPELL_ARC_WELD
-- 59085), non-triggered, urand(20000,21000) init / re-arm (:409/:440-441)
-- — zero gating legs. RenewSteel (:451) stays out: gated on
-- instance->GetGuidData(DATA_BJARNGRIM) + IsAlive (:446-449).
local function arcWeldTick(creature, guid)
    creature:CastSpell(nil, SPELL_ARC_WELD)
    schedule(guid, "arcWeld", {20000, 21000}, function()
        arcWeldTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (:424-432) — instance-gated, no bridge; the
-- engage port arms only the ungated timer arm (the thorim-colossus
-- precedent).
local function lieutenantEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcWeld", {20000, 21000}, function()
        arcWeldTick(creature, guid)
    end)
end

local function lieutenantLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function lieutenantDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_BJARNGRIM, 1, bjarngrimEnterCombat)
RegisterCreatureEvent(ENTRY_BJARNGRIM, 3, bjarngrimTargetDied)
RegisterCreatureEvent(ENTRY_BJARNGRIM, 4, bjarngrimDied)

RegisterCreatureEvent(ENTRY_STORMFORGED_LIEUTENANT, 1, lieutenantEnterCombat)
RegisterCreatureEvent(ENTRY_STORMFORGED_LIEUTENANT, 2, lieutenantLeaveCombat)
RegisterCreatureEvent(ENTRY_STORMFORGED_LIEUTENANT, 4, lieutenantDied)
RegisterCreatureEvent(ENTRY_STORMFORGED_LIEUTENANT, 23, lieutenantLeaveCombat)

-- C++ boss_bjarngrim.cpp:113/163-167/191-196: ctor canBuff = true;
-- Reset: if (canBuff) if (!HasAura(52092)) AddAura(52092, me);
-- EnterEvadeMode: canBuff = HasAura(52092) (snapshot — dispelled buff is
-- never re-applied). Nil (never evaded, incl. first spawn) reads as true
-- = the ctor state; creature:HasAura (lua_creature_events.go:141) and
-- single-arg creature:AddAura (lua_creature_events.go:170) are bridged.
local SPELL_TEMPORARY_ELECTRICAL_CHARGE = 52092
local canBuff = {}
local function bjarngrimLeaveCombat(event, creature)
    canBuff[creature:GetGUID()] = creature:HasAura(SPELL_TEMPORARY_ELECTRICAL_CHARGE)
    bjarngrimStanceCancel(creature:GetGUID())
end
local function bjarngrimReset(event, creature)
    local guid = creature:GetGUID()
    if canBuff[guid] ~= false and not creature:HasAura(SPELL_TEMPORARY_ELECTRICAL_CHARGE) then
        creature:AddAura(SPELL_TEMPORARY_ELECTRICAL_CHARGE)
    end
    bjarngrimStanceReset(creature)
end
RegisterCreatureEvent(ENTRY_BJARNGRIM, 2, bjarngrimLeaveCombat)
RegisterCreatureEvent(ENTRY_BJARNGRIM, 23, bjarngrimReset)
