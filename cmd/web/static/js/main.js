document.addEventListener('DOMContentLoaded', () => {
    const expressionInput = document.getElementById('expression');
    const calculateButton = document.getElementById('calculate-btn');
    const resultDiv = document.getElementById('result');
    const historyList = document.getElementById('history-list');

    loadHistory();

    document.addEventListener('load-history', () => {
        console.log('Получено событие load-history');
        loadHistory();
    });
    
    // Обработчик для нового события с данными истории
    document.addEventListener('load-history-data', (event) => {
        console.log('Получено событие load-history-data с данными:', event.detail);
        if (event.detail) {
            displayHistoryData(event.detail);
        }
    });

    calculateButton.addEventListener('click', () => {
        calculateExpression();
    });
    
    // Добавляем обработчик клавиш для поля ввода напрямую
    if (expressionInput) {
        expressionInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                if (typeof window.calculate === 'function') {
                    window.calculate();
                } else {
                    calculateExpression();
                }
            }
        });
    }

    // --- ДОБАВЛЯЕМ ОБРАБОТЧИКИ ДЛЯ АВТОРИЗАЦИИ ---
    const loginBtn = document.getElementById('login-btn');
    const registerBtn = document.getElementById('register-btn');
    if (loginBtn) loginBtn.addEventListener('click', login);
    if (registerBtn) registerBtn.addEventListener('click', register);

    // --- ДОБАВЛЯЕМ ОБРАБОТЧИК ДЛЯ ВЫХОДА ---
    const logoutBtn = document.getElementById('logout-btn');
    if (logoutBtn) logoutBtn.addEventListener('click', logout);

    // --- ДОБАВЛЕНО: проверка токена при загрузке страницы ---
    const token = localStorage.getItem('token');
    const username = localStorage.getItem('userLogin');
    if (token) {
        document.getElementById('auth-container').classList.add('hidden');
        document.getElementById('calculator-container').classList.remove('hidden');
        document.getElementById('history-container').classList.remove('hidden');
        if (username) {
            document.getElementById('username').textContent = username;
        }
        loadHistory();
        updateTokenExpiry && updateTokenExpiry();
    } else {
        document.getElementById('auth-container').classList.remove('hidden');
        document.getElementById('calculator-container').classList.add('hidden');
        document.getElementById('history-container').classList.add('hidden');
    }

    async function calculateExpression() {
        const expression = expressionInput.value.trim();
        if (!expression) {
            showError('Пожалуйста, введите выражение');
            return;
        }
        
        try {
            resultDiv.innerHTML = '<div class="processing">Вычисление...</div>';
            console.log('Отправка выражения на вычисление:', expression);
            
            const response = await fetch('/api/v1/calculate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + localStorage.getItem('token')
                },
                body: JSON.stringify({ expression })
            });
            
            const responseText = await response.text();
            let data;
            try {
                data = JSON.parse(responseText);
            } catch (jsonError) {
                data = null;
            }
            console.log('Ответ от сервера:', data || responseText);
            
            if (!response.ok) {
                // Если удалось распарсить JSON и есть поле error/message — показываем его, иначе показываем текст ответа
                showError((data && (data.error || data.message)) || responseText || `Ошибка: ${response.status}`);
                return;
            }
            
            if (data && data.id) {
                await pollExpressionResult(data.id);
            } else {
                showError('Не получен идентификатор выражения');
            }
        } catch (error) {
            console.error('Ошибка:', error);
            if (error.message === 'Failed to fetch') {
                showError('Сервис недоступен. Проверьте подключение или попробуйте позже.');
            } else {
            showError(error.message);
            }
        }
    }

    async function pollExpressionResult(id, maxAttempts = 20) {
        let attempts = 0;
        
        while (attempts < maxAttempts) {
            try {
                const response = await fetch(`/api/v1/expressions/${id}`, {
                    headers: {
                        'Authorization': 'Bearer ' + localStorage.getItem('token')
                    }
                });
                
                const data = await response.json();
                console.log('Статус выражения:', data);
                
                if (data.status === 'COMPLETED') {
                    if (data.result !== undefined) {
                        resultDiv.innerHTML = `<div class="success">Результат: ${data.result}</div>`;
                    } else {
                        resultDiv.innerHTML = '';
                    }
                    // Скрываем уведомление об ошибке, если оно было
                    const notification = document.getElementById('notification');
                    if (notification) notification.classList.add('hidden');
                    await loadHistory();
                    return;
                }
                
                if (data.status === 'ERROR') {
                    throw new Error('Ошибка при вычислении');
                }
                
                await new Promise(resolve => setTimeout(resolve, 1000));
                attempts++;
            } catch (error) {
                console.error('Ошибка при проверке статуса:', error);
                if (error.message === 'Failed to fetch') {
                    showError('Сервис недоступен. Проверьте подключение или попробуйте позже.');
                } else {
                    showError(error.message);
                }
                return;
            }
        }
        
        showError('Превышено время ожидания результата');
    }

    async function loadHistory() {
        if (!localStorage.getItem('token')) {
            return;
        }
        try {
            console.log('Начинаю загрузку истории...');
            const response = await fetch('/api/v1/history', {
                headers: {
                    'Authorization': 'Bearer ' + localStorage.getItem('token')
                }
            });
            
            if (!response.ok) {
                throw new Error(`Ошибка при загрузке истории: ${response.status}`);
            }
            
            const data = await response.json();
            console.log('Полученные данные:', data);
            
            // Вызываем функцию отображения данных
            displayHistoryData(data);
        } catch (error) {
            console.error('Ошибка при загрузке истории:', error);
            if (error.message === 'Failed to fetch') {
                showError('Сервис недоступен. Проверьте подключение или попробуйте позже.');
            } else {
                showError(error.message);
            }
        }
    }
    
    // Выделяем логику отображения истории в отдельную функцию
    function displayHistoryData(data) {
        console.log('Отображение данных истории (начало функции):', data);
        console.log('JSON.stringify данных истории:', JSON.stringify(data));
        console.log('Конструктор данных истории:', data.constructor.name);
        console.log('Есть ли expressions в data:', data.hasOwnProperty('expressions'));
        
        let expressions = data.expressions;
        console.log('Выделенные expressions:', expressions);
        
        // Проверка на случай, если data сам является массивом
        if (Array.isArray(data) && !expressions) {
            console.log('Data сам является массивом, используем его как expressions');
            expressions = data;
        }
        
        // Проверка на случай, если expressions не является массивом
        if (expressions && typeof expressions === 'object' && !Array.isArray(expressions)) {
            console.log('expressions является объектом, пытаемся преобразовать');
            // Если expressions - это объект, проверим, может он имеет свойство expressions или items
            if (expressions.expressions) {
                expressions = expressions.expressions;
            } else if (expressions.items) {
                expressions = expressions.items;
            } else {
                // Если это просто объект свойств, попробуем преобразовать его в массив
                try {
                    expressions = Object.values(expressions);
                } catch (error) {
                    console.error('Не удалось преобразовать объект в массив:', error);
                }
            }
        }
        
        console.log('Тип expressions после обработки:', Array.isArray(expressions) ? 'массив' : typeof expressions);
        if (expressions) {
            console.log('Длина expressions после обработки:', expressions.length);
        }
            
            const historyBody = document.getElementById('history-body');
            console.log('Элемент истории найден:', historyBody !== null);
            
            if (!historyBody) {
                console.error('Элемент history-body не найден!');
                return;
            }
            
            historyBody.innerHTML = '';
            
        if (expressions && Array.isArray(expressions) && expressions.length > 0) {
            console.log('Количество выражений:', expressions.length);
            console.log('Первое выражение:', expressions[0]);
            const sortedExpressions = [...expressions].reverse();
                console.log('Отсортированные выражения:', sortedExpressions);
            
            let expressionsAdded = 0;
                
                sortedExpressions.forEach((expr, index) => {
                    console.log(`Выражение ${index}:`, expr);
                
                // Проверка наличия необходимых полей
                if (!expr || typeof expr !== 'object') {
                    console.error(`Пропускаем выражение ${index}: неверный формат`, expr);
                    return;
                }
                
                if (!expr.text && expr.expression) {
                    expr.text = expr.expression;
                }
                
                if (!expr.text) {
                    console.error(`Пропускаем выражение ${index}: отсутствует поле text или expression`, expr);
                    return;
                }
                
                    const tr = document.createElement('tr');
                const status = ((expr.status || '')).toLowerCase();
                    tr.className = status;
                    
                    const expressionCell = document.createElement('td');
                expressionCell.textContent = expr.text || 'Неизвестное выражение';
                    
                    const resultCell = document.createElement('td');
                const isCompleted = status === 'completed';
                resultCell.textContent = isCompleted && expr.result !== undefined ? expr.result : '-';
                    
                    const statusCell = document.createElement('td');
                    let statusRu = 'В обработке';
                if (status === 'completed') statusRu = 'Готово';
                if (status === 'error') statusRu = 'Ошибка';
                    statusCell.textContent = statusRu;
                    
                    const dateCell = document.createElement('td');
                    console.log('Дата из выражения:', expr.created_at);
                    dateCell.textContent = expr.created_at || '-';
                    console.log('Текст ячейки даты:', dateCell.textContent);
                    
                    tr.appendChild(expressionCell);
                    tr.appendChild(resultCell);
                    tr.appendChild(statusCell);
                    tr.appendChild(dateCell);
                    
                    tr.addEventListener('click', () => {
                    if (expressionInput) {
                        expressionInput.value = expr.text || '';
                        if (resultDiv) {
                        resultDiv.scrollIntoView({ behavior: 'smooth' });
                        }
                    }
                    });
                    
                    historyBody.appendChild(tr);
                expressionsAdded++;
                });
            
            console.log(`Добавлено выражений в таблицу: ${expressionsAdded}`);
            } else {
            console.log('Нет выражений для отображения');
                const tr = document.createElement('tr');
                const td = document.createElement('td');
                td.colSpan = 4;
                td.textContent = 'История пуста';
                td.style.textAlign = 'center';
                tr.appendChild(td);
                historyBody.appendChild(tr);
        }
    }

    function showError(message) {
        const notification = document.getElementById('notification');
        if (notification) {
            notification.textContent = message;
            notification.className = 'notification error';
            notification.classList.remove('hidden');
        }
        if (resultDiv) resultDiv.innerHTML = '';
    }

    // --- ДОБАВЛЯЕМ ФУНКЦИЮ ОТОБРАЖЕНИЯ PROCESSING ---
    function renderProcessing(expressions) {
        const processingBody = document.getElementById('processing-body');
        const processingContainer = document.getElementById('processing-container');
        if (!expressions || expressions.length === 0) {
            processingBody.innerHTML = '<tr><td colspan="4" class="empty">Нет текущих вычислений</td></tr>';
            processingContainer.classList.add('hidden');
            return;
        }
        processingContainer.classList.remove('hidden');
        processingBody.innerHTML = '';
        expressions.forEach(expr => {
            const row = document.createElement('tr');
            row.className = 'processing';
            row.innerHTML = `
                <td>${expr.expression}</td>
                <td>${expr.result ?? ''}</td>
                <td>${expr.status}</td>
                <td>${expr.created_at}</td>
            `;
            processingBody.appendChild(row);
        });
    }

    // --- МОДИФИЦИРУЕМ ОБРАБОТКУ ИСТОРИИ ---
    document.addEventListener('load-history-data', function(e) {
        const data = e.detail;
        let expressions = [];
        if (data && Array.isArray(data.expressions)) {
            expressions = data.expressions;
        } else if (Array.isArray(data)) {
            expressions = data;
        }
        // Разделяем выражения по статусу
        const processing = expressions.filter(x => x.status === 'PROCESSING');
        const completed = expressions.filter(x => x.status !== 'PROCESSING');
        renderProcessing(processing);
        renderHistory(completed);
    });

    // --- ДОБАВЛЯЕМ АВТООБНОВЛЕНИЕ ---
    setInterval(() => {
        document.dispatchEvent(new CustomEvent('load-history'));
    }, 3000);

    // --- ОТОБРАЖЕНИЕ СРОКА ЖИЗНИ ТОКЕНА ---
    function updateTokenExpiry() {
        const expiryDiv = document.getElementById('token-expiry');
        const tokenTimestamp = localStorage.getItem('tokenTimestamp');
        if (!expiryDiv || !tokenTimestamp) {
            if (expiryDiv) expiryDiv.textContent = '';
            return;
        }
        const now = Date.now();
        const issued = parseInt(tokenTimestamp, 10);
        const expiresInMs = 60 * 60 * 1000; // 60 минут
        const expiresAt = issued + expiresInMs;
        let minutesLeft = Math.floor((expiresAt - now) / 60000);
        if (minutesLeft < 0) minutesLeft = 0;
        expiryDiv.textContent = `Срок действия токена: ${minutesLeft} мин.`;
        if (minutesLeft <= 15) {
            expiryDiv.style.color = '#f44336'; // красный
        } else {
            expiryDiv.style.color = '#222'; // чёрный
        }
    }

    // Запускаем обновление срока жизни токена каждую секунду
    setInterval(updateTokenExpiry, 1000);
    document.addEventListener('DOMContentLoaded', updateTokenExpiry);

    function login() {
        const login = document.getElementById('login').value;
        const password = document.getElementById('password').value;
        if (!login || !password) {
            showError('Введите логин и пароль');
            return;
        }
        fetch('/api/v1/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ login, password })
        })
        .then(response => {
            if (!response.ok) throw new Error('Неверные учетные данные');
            return response.json();
        })
        .then(data => {
            localStorage.setItem('token', data.token);
            localStorage.setItem('tokenTimestamp', Date.now().toString());
            localStorage.setItem('userLogin', login);
            document.getElementById('username').textContent = login;
            document.getElementById('auth-container').classList.add('hidden');
            document.getElementById('calculator-container').classList.remove('hidden');
            document.getElementById('history-container').classList.remove('hidden');
            showSuccess('Вы успешно вошли в систему');
            document.dispatchEvent(new CustomEvent('load-history'));
            updateTokenExpiry(); // обновить сразу после входа
        })
        .catch(error => {
            if (error.message === 'Failed to fetch') {
                showError('Сервис недоступен. Проверьте подключение или попробуйте позже.');
            } else {
                showError(error.message);
            }
        });
    }

    function register() {
        const login = document.getElementById('login').value;
        const password = document.getElementById('password').value;
        if (!login || !password) {
            showError('Введите логин и пароль');
            return;
        }
        fetch('/api/v1/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ login, password })
        })
        .then(async response => {
            const data = await response.json().catch(() => ({}));
            if (!response.ok) {
                // Проверяем текст ошибки
                if (data && data.error && data.error.includes('уже существует')) {
                    throw new Error('Пользователь с таким логином уже существует');
                }
                throw new Error(data.error || 'Ошибка регистрации');
            }
            return data;
        })
        .then(() => {
            showSuccess('Регистрация прошла успешно! Теперь вы можете войти.');
        })
        .catch(error => {
            if (error.message === 'Failed to fetch') {
                showError('Сервис недоступен. Проверьте подключение или попробуйте позже.');
            } else {
                showError(error.message);
            }
        });
    }

    function showSuccess(message) {
        const notification = document.getElementById('notification');
        if (notification) {
            notification.textContent = message;
            notification.className = 'notification success';
            notification.classList.remove('hidden');
        }
        if (resultDiv) resultDiv.innerHTML = '';
    }

    // Скрывать уведомление при вводе в поля логина/пароля
    const loginInput = document.getElementById('login');
    const passwordInput = document.getElementById('password');
    if (loginInput) loginInput.addEventListener('input', () => {
        const notification = document.getElementById('notification');
        if (notification) notification.classList.add('hidden');
    });
    if (passwordInput) passwordInput.addEventListener('input', () => {
        const notification = document.getElementById('notification');
        if (notification) notification.classList.add('hidden');
    });

    function logout() {
        localStorage.removeItem('token');
        localStorage.removeItem('tokenTimestamp');
        localStorage.removeItem('userLogin');
        document.getElementById('username').textContent = 'Гость';
        document.getElementById('auth-container').classList.remove('hidden');
        document.getElementById('calculator-container').classList.add('hidden');
        document.getElementById('history-container').classList.add('hidden');
        const processingContainer = document.getElementById('processing-container');
        if (processingContainer) processingContainer.classList.add('hidden');
        const resultDiv = document.getElementById('result');
        if (resultDiv) resultDiv.innerHTML = '';
        // Очищаем и скрываем уведомление об ошибке
        const notification = document.getElementById('notification');
        if (notification) {
            notification.textContent = '';
            notification.classList.add('hidden');
        }
        const expiryDiv = document.getElementById('token-expiry');
        if (expiryDiv) expiryDiv.textContent = '';
    }
}); 