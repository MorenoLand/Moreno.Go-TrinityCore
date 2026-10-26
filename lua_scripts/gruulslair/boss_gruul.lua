-- Gruul the Dragonkiller (Gruul's Lair) — Lua port of
-- src/server/scripts/Outland/GruulsLair/boss_gruul.cpp
-- (boss_gruul; the spell_gruul_shatter / spell_gruul_shatter_effect
-- SpellScripts documented below, not registered); gruuls_lair.h
-- (DATA_GRUUL = 1; note: gruuls_lair.h carries NO NPC_GRUUL entry
-- constant — Gruul's entry is a DB-side creature_template
-- ScriptName binding, no TDB in this workspace). Entry: 19044
-- Gruul the Dragonkiller — verified externally: wowhead npc=19044
-- /gruul-the-dragonkiller and tbc.cavernoftime.com/npc=19044; the
-- C++ ScriptName is "boss_gruul" per CreatureScript("boss_gruul")
-- in AddSC_boss_gruul. Talk lines used: SAY_AGGRO=0 (pull),
-- SAY_SLAY=3 (kill — player-gated in C++, C++-exact via
-- victim:GetObjectType() == "Player"), SAY_DEATH=4 (death), EMOTE_GROW=5 (growth);
-- SAY_SLAM=1 and SAY_SHATTER=2 are defined in the C++ Yells enum
-- but never called anywhere in the file — unused enum members,
-- documented only. Eluna creature events: 1 OnEnterCombat, 2
-- OnLeaveCombat, 3 OnTargetDied, 4 OnDied, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat
-- tick), like C++ DoMeleeAttackIfReady. C++ DoCast default is
-- triggered=false (vaelastrasz convention); DoCastVictim takes nil
-- as the victim arm (felmyst convention).
-- Fight shape (C++-exact for the modeled arms): OnReset(23):
-- per-GUID state cleared (the _Reset instance bookkeeping is
-- blocked on the instance-script model). OnEnterCombat(1):
-- per-GUID scheduler reset to the C++ Initialize() values
-- (growth 30s / cave-in 27s, static 30s / ground slam 35s /
-- hurtful 8s / reverberation 105s / performing=false) +
-- Talk(SAY_AGGRO) + start of the 1s scheduler pump (a port of
-- boss_gruulAI::UpdateAI — the 10s ground-slam window is modeled
-- exactly with 1s granularity; all C++ timers are whole-second
-- values except the static-timer decay, which is exact). Pump per
-- tick: growth always ticks — on fire: Talk(EMOTE_GROW) +
-- non-triggered self-cast 36300, re-arm 30s. If performing: only
-- the ground-slam timer ticks — on fire: ground slam re-arm 120s,
-- hurtful re-arm 8s, reverberation += 10s if <10s (C++-exact
-- "little time to undo the shatter damage" arm), non-triggered
-- self-cast shatter 33654, performing=false (see deviations — C++
-- clears this flag in the SpellHitTarget arm). Else: hurtful
-- strike — pick = first alive non-victim GetAITargets entry (C++
-- SelectTarget(MaxThreat, 1) — curator
-- convention, threat-sorted list); if pick exists AND gruul is within
-- melee range of his victim (creature:IsWithinMeleeRange)
-- -> DoCast(pick, 33813)
-- else DoCastVictim(33813); re-arm 8s. Reverberation — triggered
-- DoCastVictim(36297, C++-exact); re-arm {15s,25s}. Cave in —
-- DoCast(random threat-list target, 36240) (C++ SelectTarget(Random, 0),
-- playerOnly defaults FALSE, UnitAI.h:170; nil pick skips the cast
-- but still runs the static-timer arm, C++-exact); static timer:
-- if >= 4s, -= 2s (floor 2s); cave-in re-arm = static timer
-- (C++-exact). Ground slam — on fire: performing=true, timer=10s,
-- non-triggered self-cast 33525 (the MotionMaster Clear/MoveIdle
-- arms have no movement bridge — below). OnTargetDied(3): if the
-- victim is a player, Talk(SAY_SLAY) (C++-exact TYPEID gate).
-- OnDied(4): Talk(SAY_DEATH) + cleanup (the _JustDied arm is
-- blocked on the instance-script model). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state.
-- The SpellHitTarget override: the "event 15 never fires" claim is
-- STALE — event 15 (CREATURE_EVENT_ON_SPELL_HIT_TARGET) is bridged
-- (boss_ai.go:2097-2115; the arugal precedent). Ported: the shatter
-- performing-clear (gruulSpellHitTarget; the MoveChase correction leg
-- has no movement bridge and stays out). Unmodeled: the ground-slam
-- knockback arms (player hit by 33525 -> urand magnetic pull 28337 /
-- knock back 24199) — Go reports the cast target (Gruul himself on
-- the self-cast) while C++ iterates the actual AoE hit players, so the
-- player-gated arm would never fire faithfully. The two
-- SpellScripts are unmodeled (standing SpellScript gap):
-- spell_gruul_shatter (33525-hit -> RemoveAurasDueToSpell(stoned
-- 33652) + triggered 33671 shatter effect) and spell_gruul_
-- shatter_effect (distance-scaled 33671 damage).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the
-- BossAI::JustEngagedWith/JustDied/Reset DATA_GRUUL bookkeeping
-- arms skipped). The knockback arm stays out (hit-enumeration
-- gap, above); the performing-flag clear now rides event 15
-- (gruulSpellHitTarget), C++-exact; no movement bridge — the ground-slam
-- MotionMaster Clear/MoveIdle and the shatter MoveChase-resume
-- arms unmodeled (MoveIdle is NOT SetCombatMovement: the twin's own
-- note restricts it to pre-combat calls, so neither mid-combat leg
-- ports via that flag); the hurtful-strike MaxThreat
-- pick is the first alive non-victim GetAITargets entry (curator convention) and the
-- IsWithinMeleeRange gate is creature:IsWithinMeleeRange; no live UNIT_STATE_CASTING read bridge (IsCasting is constant-false — Go casts are fire-and-forget) — the
-- IsNonMeleeSpellCast queue gate unmodeled, timers fire
-- unconditionally (jeklik convention); no SpellScript bridge —
-- the shatter/shatter-effect scripts unmodeled (standing gap).

local SPELL_GROWTH = 36300
local SPELL_CAVE_IN = 36240
local SPELL_GROUND_SLAM = 33525
local SPELL_REVERBERATION = 36297
local SPELL_SHATTER = 33654
local SPELL_HURTFUL_STRIKE = 33813

local SAY_AGGRO = 0
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_GROW = 5

local ENTRY_GRUUL = 19044

local pumpTimers = {}
local gruulState = {}

-- C++ SelectTarget(Random, 0) (playerOnly defaults FALSE, UnitAI.h:170)
-- draws the threat list, not the instance player pool (thaurissan /
-- theradras / kelris precedent) — GetAITargets random pick; nil pick
-- casts nothing but keeps the static-timer arm (C++-exact).
local function randomThreatTarget(creature)
    local alive = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

local function cancelPump(guid)
    local id = pumpTimers[guid]
    if id then
        RemoveEventById(id)
        pumpTimers[guid] = nil
    end
end

local function resetState(guid)
    cancelPump(guid)
    gruulState[guid] = nil
end

-- C++ Initialize(): growth 30000, cave-in 27000, cave-in static
-- 30000, ground slam 35000, performing false, hurtful 8000,
-- reverberation 60000+45000.
local function freshState()
    return {
        growth = 30000,
        caveIn = 27000,
        caveInStatic = 30000,
        groundSlam = 35000,
        performing = false,
        hurtful = 8000,
        reverberation = 105000,
    }
end

local function gruulTick(creature, guid)
    local st = gruulState[guid]
    if not st then
        return
    end

    -- Growth ticks regardless of the ground-slam phase (C++-exact).
    st.growth = st.growth - 1000
    if st.growth <= 0 then
        creature:Talk(EMOTE_GROW)
        creature:CastSpell(creature, SPELL_GROWTH)
        st.growth = 30000
    end

    if st.performing then
        st.groundSlam = st.groundSlam - 1000
        if st.groundSlam <= 0 then
            st.groundSlam = 120000
            st.hurtful = 8000
            if st.reverberation < 10000 then
                st.reverberation = st.reverberation + 10000
            end
            creature:CastSpell(creature, SPELL_SHATTER)
            -- m_bPerformingGroundSlam clears in the SpellHitTarget(33654)
            -- arm (gruulSpellHitTarget below); the MoveChase correction
            -- leg has no movement bridge and stays out.
        end
    else
        -- Hurtful Strike: SelectTarget(MaxThreat, 1) = the first
        -- ALIVE non-victim entry of the threat-sorted GetAITargets
        -- list (C++ builds the list victim-first, so the offset pop
        -- removes the victim; offline refs skipped before the offset;
        -- curator convention); the
        -- IsWithinMeleeRange(victim) gate uses
        -- creature:IsWithinMeleeRange (gruul hurtful convention).
        st.hurtful = st.hurtful - 1000
        if st.hurtful <= 0 then
            local victimForPick = creature:GetVictim()
            local victimGuid = nil
            if victimForPick ~= nil then
                victimGuid = victimForPick:GetGUID()
            end
            local alive = {}
            for _, t in ipairs(creature:GetAITargets()) do
                if not t:IsDead() and (victimGuid == nil or t:GetGUID() ~= victimGuid) then
                    alive[#alive + 1] = t
                end
            end
            local pick = nil
            if victimGuid ~= nil then
                pick = alive[1]
            else
                pick = alive[2]
            end
            local victim = creature:GetVictim()
            if pick and victim and creature:IsWithinMeleeRange(victim) then
                creature:CastSpell(pick, SPELL_HURTFUL_STRIKE)
            else
                creature:CastSpell(nil, SPELL_HURTFUL_STRIKE)
            end
            st.hurtful = 8000
        end

        -- Reverberation: DoCastVictim(SPELL_REVERBERATION, true),
        -- C++-exact triggered cast.
        st.reverberation = st.reverberation - 1000
        if st.reverberation <= 0 then
            creature:CastSpell(nil, SPELL_REVERBERATION, true)
            st.reverberation = math.random(15000, 25000)
        end

        -- Cave In: random threat-list target (C++ SelectTarget(Random, 0),
        -- playerOnly defaults FALSE, UnitAI.h:170); nil pick skips
        -- the cast but the static-timer arm still runs (C++-exact).
        st.caveIn = st.caveIn - 1000
        if st.caveIn <= 0 then
            local pick = randomThreatTarget(creature)
            if pick then
                creature:CastSpell(pick, SPELL_CAVE_IN)
            end
            if st.caveInStatic >= 4000 then
                st.caveInStatic = st.caveInStatic - 2000
            end
            st.caveIn = st.caveInStatic
        end

        -- Ground Slam: enter the 10s performing phase (the
        -- MotionMaster Clear/MoveIdle arms have no movement
        -- bridge).
        st.groundSlam = st.groundSlam - 1000
        if st.groundSlam <= 0 then
            st.performing = true
            st.groundSlam = 10000
            creature:CastSpell(creature, SPELL_GROUND_SLAM)
        end
    end

    pumpTimers[guid] = CreateLuaEvent(function()
        gruulTick(creature, guid)
    end, 1000)
end

-- C++ JustEngagedWith: per-GUID reset + Talk(SAY_AGGRO); the
-- BossAI::JustEngagedWith instance arm is blocked.
local function gruulEnterCombat(event, creature)
    local guid = creature:GetGUID()
    resetState(guid)
    gruulState[guid] = freshState()
    creature:Talk(SAY_AGGRO)
    pumpTimers[guid] = CreateLuaEvent(function()
        gruulTick(creature, guid)
    end, 1000)
end

local function gruulLeaveCombat(event, creature)
    resetState(creature:GetGUID())
end

-- C++ boss_gruul::SpellHitTarget (:125-159): the SPELL_SHATTER arm
-- clears m_bPerformingGroundSlam when the shatter lands. Event 15
-- (CREATURE_EVENT_ON_SPELL_HIT_TARGET) fires on the caster's bindings
-- (boss_ai.go:2097-2115; the arugal precedent); the shatter cast is a
-- self-cast so the reported target is Gruul himself, but the C++
-- performing-clear does not gate on target type. The ground-slam
-- knockback arm (33525 -> 28337/24199 self-casts on the hit PLAYER)
-- stays out: Go reports the cast target (Gruul, on the self-cast)
-- while C++ iterates the actual AoE hit players — no hit-enumeration
-- bridge, and fabricating the hit set would misattribute knockbacks.
local function gruulSpellHitTarget(event, creature, target, spellId)
    if spellId == SPELL_SHATTER then
        local st = gruulState[creature:GetGUID()]
        if st and st.performing then
            st.performing = false
        end
    end
end

-- C++ KilledUnit: TYPEID_PLAYER gate (C++-exact via GetObjectType() == "Player").
local function gruulTargetDied(event, creature, victim)
    if victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ JustDied: Talk(SAY_DEATH); the _JustDied instance arm is
-- blocked.
local function gruulDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    resetState(creature:GetGUID())
end

local function gruulReset(event, creature)
    resetState(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_GRUUL, 1, gruulEnterCombat)
RegisterCreatureEvent(ENTRY_GRUUL, 2, gruulLeaveCombat)
RegisterCreatureEvent(ENTRY_GRUUL, 3, gruulTargetDied)
RegisterCreatureEvent(ENTRY_GRUUL, 4, gruulDied)
RegisterCreatureEvent(ENTRY_GRUUL, 23, gruulReset)
RegisterCreatureEvent(ENTRY_GRUUL, 15, gruulSpellHitTarget)
