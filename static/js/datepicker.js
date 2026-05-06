(function(){
    'use strict';
    var lang = document.documentElement.lang || 'fr';

    function pad(n){ return n < 10 ? '0' + n : '' + n; }

    function monthYearLabel(y, m){
        var s = new Intl.DateTimeFormat(lang, {month:'long', year:'numeric'}).format(new Date(y, m, 1));
        return s.charAt(0).toUpperCase() + s.slice(1);
    }

    var wdays = (function(){
        var out = [];
        for(var i = 0; i < 7; i++){
            var s = new Intl.DateTimeFormat(lang, {weekday:'short'}).format(new Date(2025,0,6+i));
            out.push(s.replace(/\.$/,''));
        }
        return out;
    })();

    function fmtDisplay(v){
        if(!v) return '';
        var p = v.split('-');
        if(p.length !== 3) return v;
        return new Intl.DateTimeFormat(lang, {day:'2-digit', month:'long', year:'numeric'}).format(
            new Date(parseInt(p[0]), parseInt(p[1])-1, parseInt(p[2]))
        );
    }

    function initPicker(wrap, clearLabel, todayLabel){
        if(wrap._bsdpInit) return;
        wrap._bsdpInit = true;

        var input = wrap.querySelector('input[type="hidden"]');
        var trigger = wrap.querySelector('.bsdp-trigger');
        if(!input || !trigger) return;
        var textEl = trigger.querySelector('.bsdp-trigger-text');
        var dd = document.createElement('div');
        dd.className = 'bsdp-dropdown';
        wrap.appendChild(dd);

        var now = new Date();
        var vy, vm;
        if(input.value){
            var sp = input.value.split('-');
            vy = parseInt(sp[0]); vm = parseInt(sp[1]) - 1;
        } else {
            vy = now.getFullYear(); vm = now.getMonth();
        }

        function updateText(){
            if(input.value){
                textEl.textContent = fmtDisplay(input.value);
                textEl.classList.remove('placeholder');
            } else {
                textEl.textContent = 'jj / mm / aaaa';
                textEl.classList.add('placeholder');
            }
        }

        function render(){
            var today = new Date(); today = new Date(today.getFullYear(), today.getMonth(), today.getDate());
            var first = new Date(vy, vm, 1);
            var dow = first.getDay();
            var off = (dow === 0 ? 6 : dow - 1);
            var dim = new Date(vy, vm+1, 0).getDate();
            var dimPrev = new Date(vy, vm, 0).getDate();

            var h = '<div class="bsdp-header">';
            h += '<button type="button" class="bsdp-nav bsdp-prev">\u2039</button>';
            h += '<span class="bsdp-month-year">' + monthYearLabel(vy, vm) + '</span>';
            h += '<button type="button" class="bsdp-nav bsdp-next">\u203A</button></div>';

            h += '<div class="bsdp-weekdays">';
            for(var i=0;i<7;i++) h += '<span>' + wdays[i] + '</span>';
            h += '</div><div class="bsdp-days">';

            for(var i = off-1; i >= 0; i--){
                var d = dimPrev - i;
                var pm = vm-1, py = vy; if(pm<0){pm=11;py--;}
                h += '<button type="button" class="bsdp-day other" data-d="'+py+'-'+pad(pm+1)+'-'+pad(d)+'">'+d+'</button>';
            }

            for(var d=1; d<=dim; d++){
                var ds = vy+'-'+pad(vm+1)+'-'+pad(d);
                var c = 'bsdp-day';
                if(new Date(vy,vm,d).getTime() === today.getTime()) c += ' today';
                if(input.value === ds) c += ' selected';
                h += '<button type="button" class="'+c+'" data-d="'+ds+'">'+d+'</button>';
            }

            var tot = off + dim;
            var rem = tot % 7 === 0 ? 0 : 7 - (tot % 7);
            for(var d=1; d<=rem; d++){
                var nm = vm+1, ny = vy; if(nm>11){nm=0;ny++;}
                h += '<button type="button" class="bsdp-day other" data-d="'+ny+'-'+pad(nm+1)+'-'+pad(d)+'">'+d+'</button>';
            }

            h += '</div><div class="bsdp-footer">';
            h += '<button type="button" class="bsdp-today-btn">' + todayLabel + '</button>';
            h += '<button type="button" class="bsdp-clear-btn">' + clearLabel + '</button>';
            h += '</div>';

            dd.innerHTML = h;

            dd.querySelector('.bsdp-prev').onclick = function(e){ e.stopPropagation(); vm--; if(vm<0){vm=11;vy--;} render(); };
            dd.querySelector('.bsdp-next').onclick = function(e){ e.stopPropagation(); vm++; if(vm>11){vm=0;vy++;} render(); };
            dd.querySelectorAll('.bsdp-day').forEach(function(b){
                b.onclick = function(e){
                    e.stopPropagation();
                    input.value = this.getAttribute('data-d');
                    var sp = input.value.split('-'); vy=parseInt(sp[0]); vm=parseInt(sp[1])-1;
                    updateText(); closeDd();
                };
            });
            dd.querySelector('.bsdp-today-btn').onclick = function(e){
                e.stopPropagation();
                var t = new Date();
                input.value = t.getFullYear()+'-'+pad(t.getMonth()+1)+'-'+pad(t.getDate());
                vy = t.getFullYear(); vm = t.getMonth();
                updateText(); closeDd();
            };
            dd.querySelector('.bsdp-clear-btn').onclick = function(e){
                e.stopPropagation(); input.value = ''; updateText(); closeDd();
            };
        }

        function openDd(){ render(); dd.classList.add('open'); }
        function closeDd(){ dd.classList.remove('open'); }

        trigger.addEventListener('click', function(e){
            e.stopPropagation();
            document.querySelectorAll('.bsdp-dropdown.open').forEach(function(x){ if(x!==dd) x.classList.remove('open'); });
            dd.classList.contains('open') ? closeDd() : openDd();
        });
        trigger.addEventListener('keydown', function(e){
            if(e.key==='Enter'||e.key===' '){ e.preventDefault(); trigger.click(); }
        });

        updateText();
    }

    document.addEventListener('click', function(){
        document.querySelectorAll('.bsdp-dropdown.open').forEach(function(d){ d.classList.remove('open'); });
    });

    window.initBSDatePickers = function(container){
        var root = container || document;
        var form = root.querySelector('.edit-work-form') || root.closest('.edit-work-form');
        var clearLabel = (form && form.getAttribute('data-bsdp-clear')) || 'Clear';
        var todayLabel = (form && form.getAttribute('data-bsdp-today')) || 'Today';
        root.querySelectorAll('.bsdp-wrap').forEach(function(w){ initPicker(w, clearLabel, todayLabel); });
    };

    document.addEventListener('DOMContentLoaded', function(){ window.initBSDatePickers(document); });
})();
