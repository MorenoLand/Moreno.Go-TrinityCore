-- Mechano-Lord Capacitus (The Mechanar) --
-- Lua port of src/server/scripts/Outland/TempestKeep/
-- Mechanar/boss_mechano_lord_capacitus.cpp
-- (boss_mechano_lord_capacitus — the only CreatureScript AI
-- class AddSC_boss_mechano_lord_capacitus registers).
-- Fifth unit of the Mechanar block per
-- outland_script_loader.cpp order (iron_hand, gyrokill,
-- nethermancer_sepethrea, pathaleon, capacitus,
-- instance_mechanar). Mechanar block closeout.
-- Entry: 19219 (no NPC_ constant in the C++ tree — DB-side
-- ScriptName binding, like 19218 gyrokill / 19220
-- pathaleon; entry per standard TBC creature data, not
-- verifiable from this workspace). DATA_MECHANOLORD_
-- CAPACITUS = 2 is instance-side, unbridgeable.
-- Talk: YELL_AGGRO = 0 (engage — OnEnterCombat, C++-exact),
-- YELL_REFLECTIVE_MAGIC_SHIELD = 1 (magic-shield arm —
-- C++-exact), YELL_REFLECTIVE_DAMAGE_SHIELD = 2
-- (damage-shield arm — C++-exact), YELL_KILL = 3 (kill —
-- OnTargetDied, C++-exact), YELL_DEATH = 4 (death —
-- OnDied, C++-exact).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 23 OnReset.
-- Timers via CreateLuaEvent (per-GUID countdown pump); melee
-- is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention): creature:CastSpell(nil, spell)
-- = DoCastVictim, creature:CastSpell(creature, spell) =
-- DoCast(me)/DoCastSelf; DoCastAOE = self-cast
-- (boss_ai.go DoCastAOE lowering, maiden precedent).
-- Fight shape (C++-exact for all modeled arms):
-- 1. Head Crack 35161: 10s init then 10s — non-triggered
--    DoCastVictim (C++-exact).
-- 2. Reflective Damage Shield 35159: 15s init — Talk(2) +
--    non-triggered DoCast(me); the arm then schedules
--    EVENT_REFLECTIVE_MAGIE_SHIELD at 30s (C++ enum spells
--    it MAGIE) instead of re-arming itself.
-- 3. Reflective Magic Shield 35158: Talk(1) +
--    non-triggered DoCast(me); the arm schedules
--    EVENT_REFLECTIVE_DAMAGE_SHIELD at 30s — the shields
--    ping-pong as two alternating one-shot countdowns
--    (C++-exact; exactly one shield event pending at a time).
--    No Heroic-mode gating on the C++ side of either shield.
-- 4. Polarity Shift 39096 (HEROIC ONLY — the C++
--    IsHeroic() gate is bridgeable: creature:IsHeroic() on
--    the event-handler surface, lua_creature_events.go:203):
--    15s init then randtime(45-60s) — non-triggered
--    DoCastAOE (self-cast convention, C++-exact); the
--    39096 dummy leg is PORTED via RegisterSpellEvent ev5
--    below (per-hit 50/50 self-cast of 39088/39091).
-- 5. Summon Nether Charge: 10s init — RAND of the four
--    directional summon spells 35153/35904/35905/35906,
--    non-triggered DoCastSelf; the re-arm timer is
--    DUNGEON_MODE(randtime(9-11s), randtime(2-5s)),
--    evaluated at fire time (C++-exact). NPC_NETHER_CHARGE
--    20405 has no AI class registered in the C++ file — the
--    charges' motion/immunity behavior is unbridged.
-- 6. Berserk 26662: 3min init — non-triggered DoCast(me)
--    ONE-SHOT, the C++ arm does not re-arm (C++-exact).
-- The C++ UNIT_STATE_CASTING early-return (top of UpdateAI)
-- and the post-switch cast-state re-check are PORTED
-- (creature:IsCasting, live CastingUntil window —
-- zereketh/corrupter precedent): decrement-first, pre-loop
-- gate, in-loop gate after each fired arm.
-- The polarity-charge SpellScript class in the file
-- (spell_capacitus_polarity_charge 39090/39093 stack
-- machine) stays out: the charge spells are cast by the
-- polarity-aura holder (a player) via the DBC periodic
-- trigger, and the ev5 player path has no per-effect list
-- site yet (runtime.go — same blocker as census rows
-- 525/526); secondary: PreventHitDamage has no Go model
-- and there is no GetTriggeringSpell bridge. The
-- polarity-shift dummy (39088/39091 50% roll) is PORTED
-- below via RegisterSpellEvent ev5 (census row 541); the
-- timer arm now casts 39096 DoCastAOE instead of the old
-- per-player threat-list loop.
-- OnDied(4): Talk(YELL_DEATH) (C++-exact); the BossAI
-- _JustDied arm is instance-blocked, unmodeled.
-- OnReset(23) / OnLeaveCombat(2): per-GUID state cleared
-- (the C++ Reset() / BossAI _Reset() arms are
-- instance-blocked).

local SPELL_HEADCRACK = 35161
local SPELL_REFLECTIVE_DAMAGE_SHIELD = 35159
local SPELL_REFLECTIVE_MAGIC_SHIELD = 35158
local SPELL_POLARITY_SHIFT = 39096
local SPELL_POSITIVE_POLARITY = 39088
local SPELL_NEGATIVE_POLARITY = 39091
local SPELL_BERSERK = 26662

local SUMMON_SPELLS = { 35153, 35904, 35905, 35906 }

local YELL_AGGRO = 0
local YELL_REFLECTIVE_MAGIC_SHIELD = 1
local YELL_REFLECTIVE_DAMAGE_SHIELD = 2
local YELL_KILL = 3
local YELL_DEATH = 4

local ENTRY_MECHANO_LORD_CAPACITUS = 19219

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function resetBoss(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ JustEngagedWith schedules: HEADCRACK 10s,
-- REFLECTIVE_DAMAGE_SHIELD 15s, SUMMON_NETHER_CHARGE 10s,
-- BERSERK 3min, POSITIVE_SHIFT 15s only if IsHeroic.
local function freshState(heroic)
    return {
        headcrack = 10000,
        dmgShield = 15000,
        magShield = nil,
        polarityShift = heroic and 15000 or nil,
        summon = 10000,
        berserk = 180000,
    }
end

local function bossTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end

    -- C++ events.Update(diff) advances every timer before the
    -- gate (boss_mechano_lord_capacitus.cpp:113-118):
    -- decrement first, then the pre-loop gate; expired arms
    -- stay expired for the next tick (corrupter precedent).
    st.headcrack = st.headcrack - 1000
    if st.dmgShield then
        st.dmgShield = st.dmgShield - 1000
    end
    if st.magShield then
        st.magShield = st.magShield - 1000
    end
    if st.polarityShift then
        st.polarityShift = st.polarityShift - 1000
    end
    st.summon = st.summon - 1000
    if st.berserk then
        st.berserk = st.berserk - 1000
    end
    if creature:IsCasting() then
        return
    end

    -- Head Crack 35161: 10s init then 10s — non-triggered
    -- DoCastVictim (C++-exact).
    if st.headcrack <= 0 then
        creature:CastSpell(nil, SPELL_HEADCRACK)
        st.headcrack = 10000
        if creature:IsCasting() then
            return
        end
    end

    -- Reflective Damage Shield 35159: Talk(2) + non-triggered
    -- DoCast(me); the arm schedules the magic shield at 30s
    -- instead of re-arming itself (C++-exact ping-pong).
    if st.dmgShield and st.dmgShield <= 0 then
        creature:Talk(YELL_REFLECTIVE_DAMAGE_SHIELD)
        creature:CastSpell(creature, SPELL_REFLECTIVE_DAMAGE_SHIELD)
        st.dmgShield = nil
        st.magShield = 30000
        if creature:IsCasting() then
            return
        end
    end

    -- Reflective Magic Shield 35158: Talk(1) + non-triggered
    -- DoCast(me); the arm schedules the damage shield at 30s
    -- (C++-exact ping-pong).
    if st.magShield and st.magShield <= 0 then
        creature:Talk(YELL_REFLECTIVE_MAGIC_SHIELD)
        creature:CastSpell(creature, SPELL_REFLECTIVE_MAGIC_SHIELD)
        st.magShield = nil
        st.dmgShield = 30000
        if creature:IsCasting() then
            return
        end
    end

    -- Polarity Shift 39096 (heroic only — the C++ IsHeroic()
    -- engage gate, bridgeable): 15s init then
    -- randtime(45-60s) — non-triggered DoCastAOE self-cast
    -- (C++-exact). The shift SpellScript
    -- (boss_mechano_lord_capacitus.cpp:238-264) runs per hit
    -- unit via RegisterSpellEvent ev5 below (50/50 self-cast
    -- of 39088/39091, originalCaster = Capacitus).
    if st.polarityShift and st.polarityShift <= 0 then
        creature:CastSpell(creature, SPELL_POLARITY_SHIFT)
        st.polarityShift = math.random(45000, 60000)
        if creature:IsCasting() then
            return
        end
    end

    -- Summon Nether Charge: RAND of the four directional
    -- summon spells, non-triggered DoCastSelf; re-arm is
    -- DUNGEON_MODE(randtime(9-11s), randtime(2-5s))
    -- evaluated at fire time (C++-exact).
    if st.summon <= 0 then
        creature:CastSpell(creature, SUMMON_SPELLS[math.random(1, 4)])
        if creature:IsHeroic() then
            st.summon = math.random(2000, 5000)
        else
            st.summon = math.random(9000, 11000)
        end
        if creature:IsCasting() then
            return
        end
    end

    -- Berserk 26662: 3min init, non-triggered DoCast(me)
    -- one-shot — the C++ arm does not re-arm (C++-exact).
    if st.berserk and st.berserk <= 0 then
        creature:CastSpell(creature, SPELL_BERSERK)
        st.berserk = nil
        if creature:IsCasting() then
            return
        end
    end
end

-- C++ spell_capacitus_polarity_shift::HandleDummy (boss_mechano_lord_capacitus.cpp:255-261):
-- OnEffectHitTarget, EFFECT_0 SPELL_EFFECT_DUMMY: each hit unit self-casts
-- SPELL_POSITIVE_POLARITY (39088) or SPELL_NEGATIVE_POLARITY (39091) 50/50,
-- non-triggered, originalCaster = Capacitus. ev5 fires once per (effect,
-- hit target) on the creature path (boss_ai.go:3435-3438).
-- KNOWN DELTAS: a player hit unit's self-cast rides the triggered-only
-- player funnel (lua_player.go:2897) where C++ is non-triggered, and the
-- originalCaster GUID is accepted-but-unmodeled there; a creature hit
-- unit casts non-triggered, C++-exact. DORMANT until Spell.dbc ships 39096.
local function polarityShiftDummy(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    target:CastSpell(target, math.random() < 0.5 and SPELL_POSITIVE_POLARITY or SPELL_NEGATIVE_POLARITY)
end

RegisterCreatureEvent(ENTRY_MECHANO_LORD_CAPACITUS, 1, function(_, creature)
    local guid = creature:GetGUIDLow()
    resetBoss(guid)
    states[guid] = freshState(creature:IsHeroic())
    creature:SetBossState(2, 1) -- DATA_MECHANOLORD_CAPACITUS, IN_PROGRESS (BossAI::_JustEngagedWith, ScriptedCreature.cpp:537; soccothrates precedent)
    creature:SetCombatPulseDelay(5) -- BossAI::_JustEngagedWith arm 1 (ScriptedCreature.cpp:540)
    creature:DoZoneInCombat() -- BossAI::_JustEngagedWith me-form arm (ScriptedCreature.cpp:542; sapphiron precedent)
    creature:Talk(YELL_AGGRO)
    timers[guid] = CreateLuaEvent(function()
        bossTick(creature, guid)
    end, 1000, 0)
end)

RegisterCreatureEvent(ENTRY_MECHANO_LORD_CAPACITUS, 2, function(_, creature)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_MECHANO_LORD_CAPACITUS, 3, function(_, creature)
    -- C++ KilledUnit talks unconditionally (C++-exact).
    creature:Talk(YELL_KILL)
end)

RegisterCreatureEvent(ENTRY_MECHANO_LORD_CAPACITUS, 4, function(_, creature)
    creature:SetBossState(2, 3) -- DATA_MECHANOLORD_CAPACITUS, DONE (boss_mechano_lord_capacitus.cpp:102-105: _JustDied then Talk)
    creature:Talk(YELL_DEATH)
    resetBoss(creature:GetGUIDLow())
end)

RegisterCreatureEvent(ENTRY_MECHANO_LORD_CAPACITUS, 23, function(_, creature)
    if not creature:IsDead() and creature:GetBossState(2) ~= 3 then
        creature:SetBossState(2, 0) -- DATA_MECHANOLORD_CAPACITUS, NOT_STARTED (BossAI::_Reset, ScriptedCreature.cpp:499-511; soccothrates precedent)
    end
    resetBoss(creature:GetGUIDLow())
end)
RegisterSpellEvent(SPELL_POLARITY_SHIFT, 5, polarityShiftDummy)
