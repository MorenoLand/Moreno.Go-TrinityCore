-- Leotheras the Blind (Serpentshrine Cavern) — Lua port of
-- src/server/scripts/Outland/CoilfangReservoir/SerpentShrine/
-- boss_leotheras_the_blind.cpp (boss_leotheras_the_blind,
-- boss_leotheras_the_blind_demonform, npc_greyheart_spellbinder,
-- npc_inner_demon — all four registered; AddSC_boss_leotheras_
-- the_blind registers those four C++ ScriptNames).
-- Entry 21215 verified from instance_serpent_shrine.cpp's
-- OnCreatureCreate (case 21215 -> LeotherasTheBlind, DATA_
-- LEOTHERAS); the other entries come from the file's own enum
-- constants (DEMON_FORM = 21875, NPC_SPELLBINDER = 21806, INNER_
-- DEMON_ID = 21857). The creature_template ScriptName bindings
-- are DB-side (no TDB in this workspace). Talk lines used: SAY_
-- AGGRO=0 (pull — fired by the banish-release StartEvent in C++;
-- the modeled fight starts on pull, magtheridon/ragnaros-intro
-- convention), SAY_SWITCH_TO_DEMON=1 (demon switch), SAY_INNER_
-- DEMONS=2 (inner-demon summon), SAY_DEMON_SLAY=3 / SAY_NIGHTELF_
-- SLAY=4 (kill — TYPEID_PLAYER gate, modeled via victim:
-- GetObjectType() == "Player"), SAY_FINAL_FORM=5 (15% split), SAY_FREE=6 (demon
-- copy engage), SAY_DEATH=7 (death). All eight are referenced in
-- the file.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent
-- (per-GUID scheduler pump); melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady. C++
-- DoCast default is triggered=false (vaelastrasz convention).
-- Fight shape (C++-exact for the modeled arms):
-- boss_leotheras_the_blind (21215): OnEnterCombat(1): per-GUID
-- scheduler reset to the C++ Initialize() values (whirlwind
-- 15000 / chaosBlast 1000 / switchToDemon 45000 / switchToHuman
-- 60000 / berserk 600000 / innerDemons 30000 / demonForm false /
-- isFinalForm false / needThreatReset false / enrageUsed false)
-- + Talk(SAY_AGGRO) + start of a 1s scheduler pump (a port of
-- UpdateAI — 1s granularity is exact for all C++ timers; the C++
-- banish-aura/no-victim early return collapses into the pump,
-- since the banish pre-phase and the CheckBanish poll are
-- instance/summon/cross-creature blocked — below). Pump:
-- whirlwind-aura arm: while creature:HasAura(SPELL_WHIRLWIND
-- 37640), the 2s target-switch (ResetThreatList + MovePoint to a
-- random target) is PORTED in the pump via ResetAllThreat +
-- Teleport (renataki precedent); needThreatReset arm (fires when the
-- whirlwind aura lapses or a form switches): demonForm ->
-- innerDemons=30000, else whirlwind=15000, needThreatReset=false
-- (the MotionMaster Clear/MoveChase half has no movement bridge —
-- unmodeled; the ResetThreatList half is PORTED below via
-- creature:ResetAllThreat). Berserk
-- 26662 600s one-shot (EnrageUsed-guarded): InterruptNonMeleeSpells(false)
-- then non-triggered self-cast (C++-exact, boss_leotheras_the_blind.cpp:469).
-- Nightelf form: whirlwind
-- 37640 — if the aura is absent and the timer fires -> non-
-- triggered self-cast 37640, whirlwind=2000, needThreatReset=true
-- (C++-exact); switch-to-demon (only while !isFinalForm) 45s ->
-- RemoveAura(37640), Talk(SAY_SWITCH_TO_DEMON), demonForm=true,
-- needThreatReset=true, switchToDemon=45000 (the SetDisplayId
-- MODEL_DEMON 20125 + virtual-item-slot zeroing arms are PORTED
-- 2026-10-20 — C++ arm order: display first, then Talk, then the
-- slots (SetEquipmentSlots(0, 0, 0xFFFFFFFF) leaves the ranged
-- slot untouched)). Demon form: chaos blast 37674 —
-- 1s init then 3s: if the victim is within 30 yd (GetDistance
-- bridge for the IsWithinDist gate) -> CastSpell(victim, 37674);
-- re-arm 3000 outside the range gate (C++-exact; the BP0=100
-- bonus-point arg and the original-caster arg have no bridge —
-- quake/blaze SPELLVALUE precedent; the per-tick StopMoving leg is
-- ported at pump granularity — every 1s tick with the victim in
-- range, independent of the blast timer (2026-10-22).
-- Inner demons 30s one-
-- shot -> threat-list pick of up to 5 non-victim players, per-player
-- SummonCreature(INNER_DEMON 21857) + DoZoneInCombat + player
-- AddAura(INSIDIOUS_WHISPER 37676), Talk(SAY_INNER_DEMONS), timer
-- pinned to 999999 — PORTED 2026-10-24 (the SetGUID victim binding
-- stays out, below). Switch-to-human 60s ->
-- demonForm=false, needThreatReset=true, switchToHuman=60000
-- (the CastConsumingMadness arm is demon-cast — no inner demons
-- exist in this model — and the DespawnDemon/LoadEquipment arms
-- stay out (summon/item bridges missing); the SetDisplayId
-- MODEL_NIGHTELF 20514 arm is PORTED 2026-10-20, C++ arm order
-- first) — unmodeled, below). Final
-- form: once-guarded healthBelowPct(creature, 15) (C++ HealthBelowPct(15),
-- floored integer threshold — GetHealthPct() < 15 fired up to 1 HP early)
-- -> isFinalForm=true, demonForm=false, Talk(SAY_FINAL_FORM)
-- (the SetDisplayId MODEL_NIGHTELF arm is PORTED 2026-10-20, C++
-- arm order after Talk; the DoSpawnCreature(DEMON_FORM 21875)
-- copy arm is PORTED 2026-10-24 — SummonCreature + DoZoneInCombat,
-- C++ spawn-before-Talk order; stale "no summon bridge" claim retracted).
-- OnTargetDied(3): victim:GetObjectType() == "Player"
-- -> Talk(demonForm and SAY_DEMON_SLAY or SAY_NIGHTELF_SLAY)
-- (C++-exact per-form slay). OnDied(4): Talk(SAY_DEATH) +
-- cleanup (the demon-copy despawn is cross-creature blocked; the
-- SetData(DONE) arm is instance-blocked). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state, restore the
-- nightelf model + zeroed weapon slots (PORTED 2026-10-20, C++
-- Reset :253-255 arm order; SetSpeedRate(MOVE_RUN, 2.0f) :252 PORTED
-- 2026-10-22 (creature:SetSpeedRate bridge, lua_creature_events.go:735;
-- the stale "speed blocked" claim is retracted — the speed leg is
-- separable from the unported dual-wield/corpse-delay/instance arms);
-- the CheckChannelers summon arm is PORTED 2026-10-22
-- (creature:SummonCreature bridge; Despawn stands in for the
-- unbridged DisappearAndDie, type-6 dead-despawn delta — see
-- checkChannelers); the dual-wield/corpse-delay setup,
-- SPELL_DUAL_WIELD 42459 triggered self-cast and
-- SetData(NOT_STARTED) arms stay out — item/instance blocked.
-- boss_leotheras_the_blind_demonform (21875): OnEnterCombat(1):
-- per-GUID reset (chaosBlast 1000) + Talk(SAY_FREE) (the C++
-- JustEngagedWith -> StartEvent arm) + 1s pump. Pump: chaos
-- blast — if the timer fires AND the victim is within 30 yd ->
-- CastSpell(victim, 37674), re-arm 3000 (the re-arm sits inside
-- the range gate here, unlike the boss form — C++-exact). On-
-- TargetDied(3): victim:GetObjectType() == "Player" -> Talk(SAY_DEMON_SLAY).
-- OnDied(4): cleanup (the post-mortem triggered self-cast of
-- 8149 — the blizzlike disappear — is unmodeled; a dead unit has
-- no cast bearer in the Lua model). OnLeaveCombat(2)/OnReset(23):
-- cleanup (the "deal no melee damage" arm has no bridge —
-- engine melee applies).
-- npc_greyheart_spellbinder (21806): OnEnterCombat(1): per-GUID
-- reset (mindblast {3s,8s} / earthshock {5s,10s}) + 1s pump
-- (the JustEngagedWith InterruptNonMeleeSpells leg is PORTED
-- (boss_leotheras_the_blind.cpp:731, first in binderEnterCombat);
-- the instance SetGuidData(DATA_LEOTHERAS_EVENT_STARTER) arm
-- stays instance blocked). Pump: mind blast 37531 — timer {3s,8s} init
-- then {10s,15s}: CastSpell(random alive player, 37531) (C++
-- SelectTarget(Random, 0) — no player-only gate, teron/krosh
-- convention; nil pick casts nothing, re-arm regardless —
-- C++-exact). The earthshock arm is PORTED (2026-10-22): the C++
-- scans every player for a non-null GetCurrentSpell (cast
-- detection) — bridged via player:IsCasting (lua_player.go:648) —
-- and DoCasts 39076 on the first casting player, re-arming
-- {8s,15s} regardless. The out-of-combat
-- CastChanneling arm (BANISH_BEAM 38909 on leotheras via the
-- instance DATA_LEOTHERAS GUID) and the no-event-starter evade
-- arm stay instance/cross-creature blocked (re-keyed 2026-10-24:
-- the event-7 out-of-combat pump EXISTS per the hydross/
-- dathrohan precedents — the block is the instance GUID +
-- cross-creature lookup, not the pump; the evade arm also
-- needs EnterEvadeMode, unbridged). OnDied(4):
-- cleanup (C++ JustDied is empty). OnLeaveCombat(2)/OnReset(23):
-- cleanup (the Reset re-summon relay into leotheras's
-- CheckChannelers is cross-creature blocked).
-- npc_inner_demon (21857): OnEnterCombat(1): per-GUID reset
-- (shadowBolt 10000 / link 1000) + 1s pump. Pump: soul link
-- 38007 1s — CastSpell(nil, 38007, true) triggered (the C++
-- DoCastVictim nil-victim arm — felmyst convention); demonic
-- alignment 37713 — if the creature lacks the aura ->
-- CastSpell(creature, 37713, true) triggered (C++-exact upkeep);
-- shadow bolt 39309 10s — CastSpell(nil, 39309) (C++ DoCastVictim
-- non-triggered — vaelastrasz convention). The victim binding
-- (SetGUID(INNER_DEMON_VICTIM) from leotheras — cross-creature
-- blocked), the damage-immunity arm (damage=0 unless the
-- attacker is the bound victim or self — CREATURE_EVENT_ON_-
-- DAMAGE_TAKEN (9) carries a damage rewrite, but the arm's
-- victim-GUID exemption is cross-creature blocked (ModifyThreatByPercent
-- is bridged 2026-10-24, but the binding is not), so a port would fabricate
-- blanket immunity — unmodeled) and the threat retarget/owner-
-- death
-- arms (threat-scale + AddThreat bridged 2026-10-24; the owner binding
-- (SetGUID victim) unbridged) are unmodeled; the JustDied whisper-
-- aura removal is cross-creature blocked. OnDied(4)/
-- OnLeaveCombat(2)/OnReset(23): cleanup.
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the
-- DATA_LEOTHERASTHEBLINDEVENT NOT_STARTED/IN_PROGRESS/DONE
-- bookkeeping, DATA_LEOTHERAS_EVENT_STARTER starter bookkeeping,
-- the DATA_LEOTHERAS GUID lookups and the whole banish-release
-- machine — CheckBanish's 3x SpellBinderGUID liveness poll,
-- AURA_BANISH 37833 apply/remove, ApplySpellImmune banish
-- arms, LoadEquipment, starter-victim AddThreat — skipped; the
-- banished pre-phase collapses into the pull per the
-- magtheridon convention); the 3x Greyheart Spellbinder (21806)
-- CheckChannelers summons are PORTED 2026-10-22
-- (creature:SummonCreature bridge — see checkChannelers; the
-- spellbinder-Reset re-summon relay stays cross-creature blocked);
-- the inner-demon (21857) summon arms are PORTED 2026-10-24 and the
-- final-form demon-copy (21875) spawn is PORTED 2026-10-24
-- (SummonCreature + DoZoneInCombat; the SetGUID victim-binding and
-- demon-copy despawn legs stay out); no movement/
-- threat bridge beyond ResetAllThreat/ModifyThreatByPercent/Teleport
-- — the NeedThreatReset MoveChase half (its ResetThreatList
-- half is PORTED below via creature:ResetAllThreat), the chaos-
-- blast StopMoving arm and the leotheras MoveInLineOfSight
-- intro/aggro machine unmodeled; the switch-to-demon + Reset
-- MODEL_DEMON(20125)/MODEL_NIGHTELF(20514) SetDisplayId swaps and
-- the paired UNIT_VIRTUAL_ITEM_SLOT zeroing arms are PORTED
-- 2026-10-20 (display bridge + SetEquipmentSlots bridge, C++ arm
-- order); the switch-to-human + final-form SetDisplayId arms are
-- likewise PORTED 2026-10-20 (LoadEquipment/DespawnDemon-paired
-- legs stay out — no item/summon bridges); the banish-check
-- (LoadEquipment-paired) swap, the dual-wield/corpse-delay
-- setup and the SPELL_DUAL_WIELD 42459 cast stay unmodeled —
-- summon/item/instance blocked (the 2.0f speed arm is PORTED
-- above in leotherasReset — stale "speed blocked" retracted); no
-- cross-creature bridge —
-- the spell-
-- binder leotherasGUID relay, the inner-demon victim binding,
-- the ConsumingMadness 37749 demon casts and the demon-copy
-- despawn unmodeled; the berserk InterruptNonMeleeSpells and the
-- spellbinder JustEngagedWith interrupt are PORTED (above);
-- no player-aura bridges —
-- the whisper AddAura(37676) and the cube-note player-CastSpell
-- gap (the earthshock caster scan is PORTED above via
-- player:IsCasting).

local SPELL_WHIRLWIND = 37640
local SPELL_CHAOS_BLAST = 37674
local SPELL_BERSERK = 26662
local SPELL_MINDBLAST = 37531
local SPELL_EARTHSHOCK = 39076
local SPELL_SOUL_LINK = 38007
local SPELL_SHADOWBOLT = 39309
local SPELL_DEMONIC_ALIGNMENT = 37713

local SAY_AGGRO = 0
local SAY_SWITCH_TO_DEMON = 1
local SAY_INNER_DEMONS = 2
local SPELL_INSIDIOUS_WHISPER = 37676 -- boss_leotheras_the_blind.cpp:41
local SAY_DEMON_SLAY = 3
local SAY_NIGHTELF_SLAY = 4
local SAY_FINAL_FORM = 5
local SAY_FREE = 6
local SAY_DEATH = 7

local ENTRY_LEOTHERAS = 21215
local ENTRY_DEMON_FORM = 21875
local ENTRY_SPELLBINDER = 21806
local ENTRY_INNER_DEMON = 21857

local CHAOS_BLAST_RANGE = 30

local MODEL_DEMON = 20125
local MODEL_NIGHTELF = 20514

local leotherasTimers = {}
local leotherasState = {}
local demonTimers = {}
local demonState = {}
local binderTimers = {}
local binderState = {}
local innerTimers = {}
local innerState = {}

-- Alive players sharing the creature's map+instance (teron
-- convention).
local function playersInInstance(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(Random, 0): random alive player in the instance.
local function pickRandomAlivePlayer(creature)
    local found = playersInInstance(creature)
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
end

local function cancelPump(timers, guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetCreature(timers, states, guid)
    cancelPump(timers, guid)
    states[guid] = nil
end

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() < 15 fires up to 1 HP early.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

-- C++ CheckChannelers() (boss_leotheras_the_blind.cpp:261-278):
-- per-slot x/y/o offsets from the boss's position (C++ captures
-- x,y,z at AI construction; the Lua Reset handler reads the live
-- position — exact on respawn, home on evade).
local SPELLBINDER_OFFSETS = {
    {10, -5, 2.5},
    {-8, -7, 0.9},
    {-3, 9, 5.0},
}

-- Summoned spellbinder objects per boss GUID (C++ SpellBinderGUID[3];
-- objects, not GUIDs — there is no GUID-lookup bridge).
local leotherasBinders = {}

-- C++ CheckChannelers() (boss_leotheras_the_blind.cpp:249, first arm
-- of Reset, C++ arm order): despawn the previous set, then summon 3x
-- Greyheart Spellbinder (21806) at the offsets above,
-- TEMPSUMMON_DEAD_DESPAWN (6), despawnMs 0 (C++ passes no despawn
-- time). C++ DisappearAndDie has no bridge — creature:Despawn removes
-- without a death event (documented delta); the type-6 dead-despawn is
-- likewise a documented delta (corpses take the normal sweep).
-- Skipped when the boss is dead: Go fires 23 on death, C++ Reset does
-- not run on death (ScriptedAI).
local function checkChannelers(creature, guid)
    local old = leotherasBinders[guid]
    if old then
        for _, binder in ipairs(old) do
            binder:Despawn()
        end
    end
    local binders = {}
    local hx, hy, hz = creature:GetX(), creature:GetY(), creature:GetZ()
    for _, off in ipairs(SPELLBINDER_OFFSETS) do
        local binder = creature:SummonCreature(ENTRY_SPELLBINDER,
            hx + off[1], hy + off[2], hz, off[3], 6, 0)
        if binder then
            binders[#binders + 1] = binder
        end
    end
    leotherasBinders[guid] = binders
end

-- C++ Initialize(): whirlwind 15000, chaosBlast 1000, switchToDemon
-- 45000, switchToHuman 60000, berserk 600000, innerDemons 30000,
-- demonForm/isFinalForm/needThreatReset/enrageUsed false.
local function freshLeotherasState()
    return {
        whirlwind = 15000,
        chaosBlast = 1000,
        switchToDemon = 45000,
        switchToHuman = 60000,
        berserk = 600000,
        innerDemons = 30000,
        demonForm = false,
        isFinalForm = false,
        needThreatReset = false,
        enrageUsed = false,
    }
end

local function leotherasTick(creature, guid)
    local st = leotherasState[guid]
    if not st then
        return
    end

    -- NeedThreatReset arm: fires once the whirlwind aura lapses or
    -- a form switches (the ResetThreatList half is PORTED below via
    -- creature:ResetAllThreat; the MotionMaster Clear/MoveChase half
    -- is subsumed by the engine's combat chase — the victim is
    -- re-acquired by the combat tick after the threat reset).
    if st.needThreatReset and not creature:HasAura(SPELL_WHIRLWIND) then
        if st.demonForm then
            st.innerDemons = 30000
        else
            st.whirlwind = 15000
        end
        st.needThreatReset = false
        -- C++ NeedThreatReset arm (leotheras :461): ResetThreatList();
        -- the MoveChase half is subsumed by the engine combat chase.
        creature:ResetAllThreat()
    end

    -- Berserk: 10min one-shot, EnrageUsed-guarded (C++-exact),
    -- non-triggered self-cast. C++ fires at Berserk_Timer < diff
    -- (strict, boss_leotheras_the_blind.cpp:467): at 1s pump the
    -- 600000 timer reaches exactly 1000 then 0, firing on the 0
    -- tick — `<= 1000` fired ~1s early.
    if st.berserk < 1000 and not st.enrageUsed then
        creature:InterruptNonMeleeSpells(false) -- boss_leotheras_the_blind.cpp:469
        creature:CastSpell(creature, SPELL_BERSERK)
        st.enrageUsed = true
    elseif st.berserk >= 1000 then
        st.berserk = st.berserk - 1000
    end

    if not st.demonForm then
        -- Whirlwind: cast once the aura is absent and the timer
        -- fires; while the aura holds, the C++ 2s target-switch
        -- (ResetThreatList + MovePoint to a random target,
        -- boss_leotheras_the_blind.cpp:435-447) is PORTED below:
        -- the timer resets to 2000 even on a nil pick, and the
        -- MovePoint rides the Teleport bridge (renataki precedent;
        -- Teleport sets Moving=false, subsuming the Clear leg).
        if creature:HasAura(SPELL_WHIRLWIND) then
            if st.whirlwind <= 1000 then
                local target = pickRandomAlivePlayer(creature)
                if target then
                    creature:ResetAllThreat()
                    creature:Teleport(target:GetX(), target:GetY(), target:GetZ())
                end
                st.whirlwind = 2000
            else
                st.whirlwind = st.whirlwind - 1000
            end
        else
            if st.whirlwind <= 1000 then
                creature:CastSpell(creature, SPELL_WHIRLWIND)
                st.whirlwind = 2000
                st.needThreatReset = true
            else
                st.whirlwind = st.whirlwind - 1000
            end
        end

        -- Switch to demon form (nightelf form only, never after
        -- the final form).
        if not st.isFinalForm then
            if st.switchToDemon <= 1000 then
                creature:RemoveAura(SPELL_WHIRLWIND)
                -- C++ :495: SetDisplayId(MODEL_DEMON) — first arm at the switch.
                creature:SetDisplayId(MODEL_DEMON)
                creature:Talk(SAY_SWITCH_TO_DEMON)
                -- C++ :497-498: zero the virtual item slots (paired with the
                -- display swap, C++ arm order after Talk); 0xFFFFFFFF leaves
                -- the ranged slot untouched.
                creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
                st.demonForm = true
                st.needThreatReset = true
                st.switchToDemon = 45000
            else
                st.switchToDemon = st.switchToDemon - 1000
            end
        end
    else
        -- C++ demon branch (boss_leotheras_the_blind.cpp:503-505):
        -- no victim -> return before chaos/inner/switch AND before
        -- the final-form check below (PORTED 2026-10-25; the old
        -- nil-guarded fall-through let the switch/final arms run
        -- victimless, which C++ never does).
        if not creature:GetVictim() then
            return
        end
        -- Chaos blast: 1s init then 3s — cast only when the victim
        -- is within 30 yd; re-arm sits outside the range gate
        -- (C++-exact). The StopMoving leg fires on EVERY tick the
        -- victim is in range, independent of the blast timer
        -- (boss_leotheras_the_blind.cpp:505-506). Deviation: the C++
        -- cast at :519 carries CastSpellExtraArgs().SetOriginalCaster(
        -- me->GetGUID()).AddSpellBP0(100) — the BP0 override and the
        -- original-caster tag have no Lua bridge (Lua CastSpell carries
        -- only the triggered flag; the engine cast path has no
        -- base-point-override parameter), so the port fires the plain
        -- DBC-base-point cast.
        local victim = creature:GetVictim()
        if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
            creature:StopMoving()
        end
        if st.chaosBlast <= 1000 then
            if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
                creature:CastSpell(victim, SPELL_CHAOS_BLAST)
            end
            st.chaosBlast = 3000
        else
            st.chaosBlast = st.chaosBlast - 1000
        end

        -- Inner demons: 30s one-shot, then pinned (C++-exact).
        -- C++ boss_leotheras_the_blind.cpp:524-562: threat-sorted
        -- list, players only, exclude the current victim (GetVictim
        -- stands in for GetLastVictim), cap 5, alive only; per
        -- player: SummonCreature(INNER_DEMON 21857, px+10, py+10,
        -- pz, o=0, type 3, 5000ms) + DoZoneInCombat (the AttackStart
        -- substitute — opera/broggok precedent) + player AddAura
        -- (INSIDIOUS_WHISPER 37676). The SetGUID(INNER_DEMON_VICTIM)
        -- binding stays out (no cross-creature GUID bridge), so the
        -- demon's victim-only damage immunity stays unmodeled.
        if st.innerDemons <= 1000 then
            local victim = creature:GetVictim()
            local victimGuid = victim and victim:GetGUID() or nil
            local picked = 0
            for _, target in ipairs(creature:GetAITargets()) do
                if picked >= 5 then
                    break
                end
                if target:GetObjectType() == "Player"
                        and target:GetGUID() ~= victimGuid
                        and not target:IsDead() then
                    local demon = creature:SummonCreature(ENTRY_INNER_DEMON,
                        target:GetX() + 10, target:GetY() + 10,
                        target:GetZ(), 0, 3, 5000)
                    if demon then
                        demon:DoZoneInCombat()
                    end
                    target:AddAura(SPELL_INSIDIOUS_WHISPER)
                    picked = picked + 1
                end
            end
            creature:Talk(SAY_INNER_DEMONS)
            st.innerDemons = 999999
        else
            st.innerDemons = st.innerDemons - 1000
        end

        -- Switch back to nightelf form (the CastConsumingMadness and
        -- DespawnDemon arms are demon-cast/summon blocked; the
        -- LoadEquipment weapon-restore leg has no item bridge —
        -- header).
        if st.switchToHuman <= 1000 then
            -- C++ :568: SetDisplayId(MODEL_NIGHTELF) — first arm at the
            -- switch; the LoadEquipment weapon-restore leg stays out.
            creature:SetDisplayId(MODEL_NIGHTELF)
            st.demonForm = false
            st.needThreatReset = true
            st.switchToHuman = 60000
        else
            st.switchToHuman = st.switchToHuman - 1000
        end
    end

    -- Final form: once-guarded <15% health (C++ HealthBelowPct(15)).
    -- C++ boss_leotheras_the_blind.cpp:580-599 order: CastConsumingMadness
    -- (demon-cast, no demons in this model — out) + DespawnDemon (out) +
    -- DoSpawnCreature(DEMON_FORM 21875, at the boss position, type 3,
    -- 6000ms) + AttackStart(victim) -> SummonCreature + DoZoneInCombat
    -- (the AttackStart substitute — inner-demon precedent), then the
    -- final-form flag, Talk, SetDisplayId.
    if not st.isFinalForm and healthBelowPct(creature, 15) then
        local copy = creature:SummonCreature(ENTRY_DEMON_FORM,
            creature:GetX(), creature:GetY(), creature:GetZ(),
            creature:GetO(), 3, 6000)
        if copy then
            copy:DoZoneInCombat()
        end
        st.isFinalForm = true
        st.demonForm = false
        creature:Talk(SAY_FINAL_FORM)
        -- C++ :597: SetDisplayId(MODEL_NIGHTELF) after Talk (the paired
        -- LoadEquipment weapon-restore leg has no item bridge; the
        -- DespawnDemon arm stays out — no demon GUID is ever stored).
        creature:SetDisplayId(MODEL_NIGHTELF)
    end
end

local function leotherasEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(leotherasTimers, leotherasState, guid)
    leotherasState[guid] = freshLeotherasState()
    creature:Talk(SAY_AGGRO)
    leotherasTimers[guid] = CreateLuaEvent(function()
        leotherasTick(creature, guid)
    end, 1000, 0)
end

local function leotherasLeaveCombat(event, creature)
    resetCreature(leotherasTimers, leotherasState, creature:GetGUID())
end

-- C++ KilledUnit: TYPEID_PLAYER gate -> per-form slay line.
local function leotherasTargetDied(event, creature, victim)
    local st = leotherasState[creature:GetGUID()]
    if victim:GetObjectType() == "Player" then
        if st and st.demonForm then
            creature:Talk(SAY_DEMON_SLAY)
        else
            creature:Talk(SAY_NIGHTELF_SLAY)
        end
    end
end

local function leotherasDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetCreature(leotherasTimers, leotherasState, creature:GetGUID())
end

local function leotherasReset(event, creature)
    local guid = creature:GetGUID()
    -- C++ Reset() :249 CheckChannelers() — first arm, before
    -- Initialize() (C++ arm order).
    if not creature:IsDead() then
        checkChannelers(creature, guid)
    end
    resetCreature(leotherasTimers, leotherasState, guid)
    -- C++ Reset() :252-255 runs on evade: 2.0x run speed (C++ arm order:
    -- speed first), nightelf model, zeroed weapon slots (display then the
    -- item slots); the dual-wield/corpse-delay/instance arms stay
    -- unmodeled, header.
    creature:SetSpeedRate(1, 2.0) -- MOVE_RUN, boss_leotheras_the_blind.cpp:252
    creature:SetDisplayId(MODEL_NIGHTELF)
    creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
end

RegisterCreatureEvent(ENTRY_LEOTHERAS, 1, leotherasEnterCombat)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 2, leotherasLeaveCombat)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 3, leotherasTargetDied)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 4, leotherasDied)
RegisterCreatureEvent(ENTRY_LEOTHERAS, 23, leotherasReset)

-- boss_leotheras_the_blind_demonform (21875) — see header.

-- C++ Initialize(): chaosBlast 1000.
local function freshDemonState()
    return {
        chaosBlast = 1000,
    }
end

local function demonTick(creature, guid)
    local st = demonState[guid]
    if not st then
        return
    end

    -- Chaos blast: 1s init then 3s — cast AND re-arm only when the
    -- victim is within 30 yd (the re-arm sits inside the range
    -- gate here, unlike the boss form — C++-exact). The StopMoving
    -- leg fires on EVERY tick the victim is in range, independent
    -- of the blast timer (boss_leotheras_the_blind.cpp:665-667).
    -- Deviation: same as the boss form — the C++ cast at :675 drops
    -- its CastSpellExtraArgs().SetOriginalCaster(me->GetGUID()).
    -- AddSpellBP0(100) legs (no BP0 / original-caster bridge on the
    -- Lua CastSpell surface).
    local victim = creature:GetVictim()
    if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
        creature:StopMoving()
    end
    if st.chaosBlast <= 1000 then
        if victim and creature:GetDistance(victim) <= CHAOS_BLAST_RANGE then
            creature:CastSpell(victim, SPELL_CHAOS_BLAST)
            st.chaosBlast = 3000
        end
    else
        st.chaosBlast = st.chaosBlast - 1000
    end
end

local function demonEnterCombat(event, creature)
    creature:SetMeleeAttack(false) -- boss_leotheras_the_blind.cpp:660: custom UpdateAI never calls DoMeleeAttackIfReady (C++ comments "Do NOT deal any melee damage to the target")
    local guid = creature:GetGUID()
    resetCreature(demonTimers, demonState, guid)
    demonState[guid] = freshDemonState()
    creature:Talk(SAY_FREE)
    demonTimers[guid] = CreateLuaEvent(function()
        demonTick(creature, guid)
    end, 1000, 0)
end

local function demonCleanup(event, creature)
    resetCreature(demonTimers, demonState, creature:GetGUID())
end

-- C++ KilledUnit: TYPEID_PLAYER gate -> Talk(SAY_DEMON_SLAY).
local function demonTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_DEMON_SLAY)
    end
end

RegisterCreatureEvent(ENTRY_DEMON_FORM, 1, demonEnterCombat)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 2, demonCleanup)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 3, demonTargetDied)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 4, demonCleanup)
RegisterCreatureEvent(ENTRY_DEMON_FORM, 23, demonCleanup)

-- npc_greyheart_spellbinder (21806) — see header.

-- C++ Initialize(): mindblast urand(3000,8000), earthshock
-- urand(5000,10000).
local function freshBinderState()
    return {
        mindblast = math.random(3000, 8000),
        earthshock = math.random(5000, 10000),
    }
end

local function binderTick(creature, guid)
    local st = binderState[guid]
    if not st then
        return
    end

    -- Mind blast: {3s,8s} init then {10s,15s} — random alive
    -- player pick (C++ SelectTarget(Random, 0)); nil pick casts
    -- nothing, timer re-arms (C++-exact).
    if st.mindblast <= 1000 then
        local pick = pickRandomAlivePlayer(creature)
        if pick then
            creature:CastSpell(pick, SPELL_MINDBLAST)
        end
        st.mindblast = math.random(10000, 15000)
    else
        st.mindblast = st.mindblast - 1000
    end

    -- Earthshock arm (boss_leotheras_the_blind.cpp:786-804): the C++
    -- scans the map player list for the first player with any
    -- non-null GetCurrentSpell (cast detection) and DoCasts 39076
    -- on it (non-triggered, UnitAI.h:319 default args), then re-arms
    -- {8s,15s} regardless. Ported via the live player:IsCasting
    -- bridge (lua_player.go:648, s.activeCast); the scan rides the
    -- file's playersInInstance helper (alive, same map+instance —
    -- a corpse holds no in-flight cast, so the alive filter is
    -- unobservable vs the C++ no-alive-gate scan; list order is
    -- the world's, not the map's internal order — first-caster
    -- pick keeps the anti-caster intent).
    if st.earthshock <= 1000 then
        for _, p in ipairs(playersInInstance(creature)) do
            if p:IsCasting() then
                creature:CastSpell(p, SPELL_EARTHSHOCK)
                break
            end
        end
        st.earthshock = math.random(8000, 15000)
    else
        st.earthshock = st.earthshock - 1000
    end
end

local function binderEnterCombat(event, creature)
    creature:InterruptNonMeleeSpells(false) -- boss_leotheras_the_blind.cpp:731
    local guid = creature:GetGUID()
    resetCreature(binderTimers, binderState, guid)
    binderState[guid] = freshBinderState()
    binderTimers[guid] = CreateLuaEvent(function()
        binderTick(creature, guid)
    end, 1000, 0)
end

local function binderCleanup(event, creature)
    resetCreature(binderTimers, binderState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_SPELLBINDER, 1, binderEnterCombat)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 2, binderCleanup)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 4, binderCleanup)
RegisterCreatureEvent(ENTRY_SPELLBINDER, 23, binderCleanup)

-- npc_inner_demon (21857) — see header.

-- C++ Initialize(): shadowBolt 10000, link 1000.
local function freshInnerState()
    return {
        shadowBolt = 10000,
        link = 1000,
    }
end

local function innerTick(creature, guid)
    local st = innerState[guid]
    if not st then
        return
    end

    -- C++ inner-demon UpdateAI (boss_leotheras_the_blind.cpp:169-171):
    -- no victim -> return before the link/alignment/shadowbolt arms
    -- (PORTED 2026-10-25; the retarget block above those arms stays
    -- binding-blocked, header).
    if not creature:GetVictim() then
        return
    end

    -- Soul link 1s: triggered DoCastVictim (felmyst convention —
    -- nil takes the victim arm).
    if st.link <= 1000 then
        creature:CastSpell(nil, SPELL_SOUL_LINK, true)
        st.link = 1000
    else
        st.link = st.link - 1000
    end

    -- Demonic alignment upkeep: re-cast while the aura is absent
    -- (C++-exact), triggered.
    if not creature:HasAura(SPELL_DEMONIC_ALIGNMENT) then
        creature:CastSpell(creature, SPELL_DEMONIC_ALIGNMENT, true)
    end

    -- Shadow bolt 10s: non-triggered DoCastVictim.
    if st.shadowBolt <= 1000 then
        creature:CastSpell(nil, SPELL_SHADOWBOLT)
        st.shadowBolt = 10000
    else
        st.shadowBolt = st.shadowBolt - 1000
    end
end

local function innerEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetCreature(innerTimers, innerState, guid)
    innerState[guid] = freshInnerState()
    innerTimers[guid] = CreateLuaEvent(function()
        innerTick(creature, guid)
    end, 1000, 0)
end

local function innerCleanup(event, creature)
    resetCreature(innerTimers, innerState, creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_INNER_DEMON, 1, innerEnterCombat)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 2, innerCleanup)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 4, innerCleanup)
RegisterCreatureEvent(ENTRY_INNER_DEMON, 23, innerCleanup)
