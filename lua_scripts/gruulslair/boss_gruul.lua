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
-- victim:IsPlayer()), SAY_DEATH=4 (death), EMOTE_GROW=5 (growth);
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
-- strike — pick = random alive player in the instance (C++
-- SelectTarget(MaxThreat, 1), second-highest threat — curator
-- convention, no threat model); if pick exists AND gruul is within
-- melee range of his victim (5-yd GetDistance stand-in for
-- IsWithinMeleeRange, venoxis convention) -> DoCast(pick, 33813)
-- else DoCastVictim(33813); re-arm 8s. Reverberation — triggered
-- DoCastVictim(36297, C++-exact); re-arm {15s,25s}. Cave in —
-- DoCast(random alive player, 36240) (C++ SelectTarget(Random, 0),
-- player-only default — teron convention; nil pick skips the cast
-- but still runs the static-timer arm, C++-exact); static timer:
-- if >= 4s, -= 2s (floor 2s); cave-in re-arm = static timer
-- (C++-exact). Ground slam — on fire: performing=true, timer=10s,
-- non-triggered self-cast 33525 (the MotionMaster Clear/MoveIdle
-- arms have no movement bridge — below). OnTargetDied(3): if the
-- victim is a player, Talk(SAY_SLAY) (C++-exact TYPEID gate).
-- OnDied(4): Talk(SAY_DEATH) + cleanup (the _JustDied arm is
-- blocked on the instance-script model). OnLeaveCombat(2)/
-- OnReset(23): cancel the pump, drop per-GUID state.
-- The SpellHitTarget override (event 15 exists as a scripting-
-- package constant but the world engine never fires it — eredar-
-- twins standing gap) is unmodeled: the ground-slam knockback
-- arms (player hit by 33525 -> urand magnetic pull 28337 /
-- knock back 24199) and the shatter-movement-correction arm
-- (hit by 33654 while performing -> performing=false + MoveChase
-- resume when not already chasing) have no bridge. The two
-- SpellScripts are unmodeled (standing SpellScript gap):
-- spell_gruul_shatter (33525-hit -> RemoveAurasDueToSpell(stoned
-- 33652) + triggered 33671 shatter effect) and spell_gruul_
-- shatter_effect (distance-scaled 33671 damage).
-- Deliberate deviations (all await engine bridges): no instance-
-- script model — boss admission via the luaBossAI shim (the
-- BossAI::JustEngagedWith/JustDied/Reset DATA_GRUUL bookkeeping
-- arms skipped); no SpellHitTarget bridge — the knockback arm
-- and the C++-side performing-flag clear are unmodeled, so the
-- Lua port clears performing=false inline in the 10s ground-slam
-- timer arm (documented — the flag cannot otherwise clear since
-- event 15 never fires); no movement bridge — the ground-slam
-- MotionMaster Clear/MoveIdle and the shatter MoveChase-resume
-- arms unmodeled; no threat bridge — the hurtful-strike MaxThreat
-- pick is a random alive player (curator convention) and the
-- IsWithinMeleeRange gate is a 5-yd GetDistance stand-in
-- (venoxis convention); no UNIT_STATE bridge — the
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

local function pickRandomPlayer(creature)
    local players = playersInInstance(creature)
    if #players == 0 then
        return nil
    end
    return players[math.random(#players)]
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
            -- Deviation: C++ clears m_bPerformingGroundSlam in the
            -- SpellHitTarget(33654) arm; event 15 never fires in
            -- the Go engine, so the flag is cleared here.
            st.performing = false
        end
    else
        -- Hurtful Strike: SelectTarget(MaxThreat, 1) approximated
        -- by a random alive player (curator convention); the
        -- IsWithinMeleeRange(victim) gate approximated by a 5-yd
        -- GetDistance stand-in (venoxis convention).
        st.hurtful = st.hurtful - 1000
        if st.hurtful <= 0 then
            local pick = pickRandomPlayer(creature)
            local victim = creature:GetVictim()
            if pick and victim and creature:GetDistance(victim) <= 5 then
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

        -- Cave In: random alive player (C++ SelectTarget(Random, 0),
        -- player-only default — teron convention); nil pick skips
        -- the cast but the static-timer arm still runs (C++-exact).
        st.caveIn = st.caveIn - 1000
        if st.caveIn <= 0 then
            local pick = pickRandomPlayer(creature)
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

-- C++ KilledUnit: TYPEID_PLAYER gate (C++-exact via IsPlayer()).
local function gruulTargetDied(event, creature, victim)
    if victim:IsPlayer() then
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
